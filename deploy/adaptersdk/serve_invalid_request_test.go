package adaptersdk

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// CodeInvalidRequest was declared and never sent: a request with no method or
// the wrong protocol version fell through to "method not found".
func TestServeIO_InvalidRequest(t *testing.T) {
	for name, line := range map[string]string{
		"wrong version": `{"jsonrpc":"1.0","method":"get_provider_info","id":1}`,
		"no version":    `{"method":"get_provider_info","id":1}`,
		"no method":     `{"jsonrpc":"2.0","id":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := ServeIO(newFakeProvider(), strings.NewReader(line+"\n"), &out); err != nil {
				t.Fatalf("ServeIO error: %v", err)
			}
			var resp response
			if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
				t.Fatalf("want error code %d, got %+v", CodeInvalidRequest, resp.Error)
			}
			if string(resp.ID) != "1" {
				t.Errorf("want the request id echoed, got %s", resp.ID)
			}
		})
	}
}
