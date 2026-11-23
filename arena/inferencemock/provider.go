// Package inferencemock is a keyless stand-in for a role: inference provider,
// so a kit whose checks call a classifier (topic_policy, the text classifiers)
// runs in CI the way a type: mock LLM provider lets the agent run.
//
// It plays the classifier, not the check: the check still renders its policy,
// sends the turn, and applies its own allow/deny rules to the label it gets
// back. The mock picks that label from ordered rules over the message text:
//
//	providers:
//	  - id: topic-classifier
//	    type: mock
//	    role: inference
//	    additional_config:
//	      rules:
//	        - match: "(?i)order|parcel|refund"
//	          label: on-topic
//	      default_label: off-topic
//
// The first rule whose regular expression matches the newest input wins; with
// none matching, default_label applies. A label the check did not offer (or no
// label at all) comes back with no scores, which every check treats as "no
// usable answer" and routes through its own policy for that case.
package inferencemock

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/AltairaLabs/PromptKit/runtime/v2/inference"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// ProviderType is the `type` a provider file declares alongside role: inference.
const ProviderType = "mock"

//nolint:gochecknoinits // Factory registration requires init.
func init() {
	inference.RegisterFactory(ProviderType, func(spec inference.ProviderSpec) (inference.Provider, error) {
		return New(spec.AdditionalConfig)
	})
}

type rule struct {
	match *regexp.Regexp
	label string
}

// Provider answers inference requests from fixed rules.
type Provider struct {
	rules        []rule
	defaultLabel string
}

// New builds a Provider from a provider file's additional_config.
func New(cfg map[string]any) (*Provider, error) {
	p := &Provider{}
	if d, ok := cfg["default_label"]; ok {
		s, isString := d.(string)
		if !isString {
			return nil, fmt.Errorf("inferencemock: default_label must be a string, got %T", d)
		}
		p.defaultLabel = s
	}
	raw, ok := cfg["rules"]
	if !ok {
		if p.defaultLabel == "" {
			return nil, errors.New("inferencemock: set rules, default_label, or both")
		}
		return p, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("inferencemock: rules must be a list, got %T", raw)
	}
	for i, item := range list {
		r, err := parseRule(item)
		if err != nil {
			return nil, fmt.Errorf("inferencemock: rules[%d]: %w", i, err)
		}
		p.rules = append(p.rules, r)
	}
	return p, nil
}

func parseRule(item any) (rule, error) {
	m, ok := item.(map[string]any)
	if !ok {
		return rule{}, fmt.Errorf("must be a map with match and label, got %T", item)
	}
	pattern, _ := m["match"].(string)
	label, _ := m["label"].(string)
	if pattern == "" || label == "" {
		return rule{}, errors.New("needs both match and label")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return rule{}, fmt.Errorf("match: %w", err)
	}
	return rule{match: re, label: label}, nil
}

// Infer labels the newest input. The chosen label scores 1 and every other
// offered label 0.
func (p *Provider) Infer(_ context.Context, req inference.Request) (inference.Response, error) {
	label := p.label(newestText(req.Inputs))
	resp := inference.Response{Model: ProviderType, Raw: label}
	if !offered(label, req.Labels) {
		return resp, nil
	}
	resp.Scores = append(resp.Scores, inference.LabelScore{Label: label, Score: 1})
	for _, l := range req.Labels {
		if !strings.EqualFold(l, label) {
			resp.Scores = append(resp.Scores, inference.LabelScore{Label: l, Score: 0})
		}
	}
	return resp, nil
}

func (p *Provider) label(text string) string {
	for _, r := range p.rules {
		if r.match.MatchString(text) {
			return r.label
		}
	}
	return p.defaultLabel
}

// newestText is the message under judgment: checks send recent context first
// and the message they are deciding on last.
func newestText(inputs []types.Message) string {
	if len(inputs) == 0 {
		return ""
	}
	return inputs[len(inputs)-1].GetContent()
}

// offered reports whether label is one the check asked about. An empty label
// set means the caller accepts any label.
func offered(label string, labels []string) bool {
	if label == "" {
		return false
	}
	if len(labels) == 0 {
		return true
	}
	for _, l := range labels {
		if strings.EqualFold(l, label) {
			return true
		}
	}
	return false
}
