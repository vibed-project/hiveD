package cmd

import "fmt"

// resolveKind maps a CLI-friendly alias (as typed after `get`/`watch`) to
// the canonical resource kind name used on the wire and in manifests.
func resolveKind(alias string) (string, error) {
	switch alias {
	case "colony", "colonies", "co":
		return "Colony", nil
	case "agent", "agents", "ag":
		return "Agent", nil
	case "agentversion", "agentversions", "av":
		return "AgentVersion", nil
	case "run", "runs":
		return "Run", nil
	case "tool", "tools":
		return "Tool", nil
	default:
		return "", fmt.Errorf("unknown resource kind %q (want colony, agent, agentversion, run, or tool)", alias)
	}
}
