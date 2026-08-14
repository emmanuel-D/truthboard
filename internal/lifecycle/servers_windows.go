//go:build windows

package lifecycle

// mcpProcesses has no Windows answer yet: there is no `ps` to ask, and a
// wrong list here would be worse than none — it would name processes to
// restart that are already current. Silence, the same answer the JetBrains
// check and spawnWarning give on the platform they cannot advise on. The
// server's own in-band warning is unaffected: it compares its build against
// PATH and works everywhere.
func mcpProcesses() []mcpProcess { return nil }
