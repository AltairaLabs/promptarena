package arenaconfig

// MemoryEnabled reports whether the config turns the memory capability on.
// A memory block turns it on unless it says `enabled: false`; a block that
// leaves `enabled` out stays on, as it always has. `memory: false` also turns
// it off. No block means no memory.
func (c *Config) MemoryEnabled() bool {
	switch m := c.Memory.(type) {
	case nil:
		return false
	case bool:
		return m
	case map[string]interface{}:
		return enabledFlag(m["enabled"])
	case map[interface{}]interface{}:
		return enabledFlag(m["enabled"])
	default:
		return true
	}
}

// enabledFlag reads an `enabled` value: only an explicit false turns it off.
func enabledFlag(v interface{}) bool {
	b, ok := v.(bool)
	return !ok || b
}
