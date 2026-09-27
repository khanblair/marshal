package security

import (
	"path/filepath"
	"strings"
)

// DeployWorkflow says whether a command line runs a deploy workflow
// (docs/backend-checklist.md B3.7, docs/marshal-product-scope.md section 14.8). A deploy needs a
// person in every mode except bypass, so this is checked before the mode is asked what to do.
//
// It reads the command line, not the agent's description of it: the tool it names, and for a tool
// that does more than deploy, the subcommand it is given. A tool this package does not know is not
// a deploy, because calling every unknown command a deploy would make the rule useless. Every
// command in the line is read, so a deploy chained behind another command ("cd infra && terraform
// apply") is still a deploy.
func DeployWorkflow(command string) bool {
	for _, segment := range Segments(command) {
		if deploysInSegment(segment) {
			return true
		}
	}
	return false
}

// deploysInSegment says whether one command runs a deploy.
func deploysInSegment(segment string) bool {
	args := commandTail(Words(segment))
	if len(args) == 0 {
		return false
	}
	tool := filepath.Base(args[0])
	rest := args[1:]
	if deployTools[tool] {
		return true
	}
	if verbs, known := deployVerbs[tool]; known {
		return anyVerb(rest, verbs)
	}
	if runnerTools[tool] {
		return anyVerb(skipRunner(rest), deployVerbsEverywhere)
	}
	return false
}

// deployTools are commands whose every use is a deploy.
var deployTools = map[string]bool{
	"vercel": true, "netlify": true, "heroku": true, "fly": true, "flyctl": true,
	"ansible-playbook": true, "cap": true, "eb": true, "serverless": true, "sls": true,
	"wrangler": true,
}

// deployVerbs maps a tool to the subcommands that run a deploy. A tool listed here that is given
// any other subcommand is not deploying.
var deployVerbs = map[string]map[string]bool{
	"terraform": {"apply": true, "destroy": true, "import": true},
	"kubectl": {
		"apply": true, "create": true, "delete": true, "replace": true, "patch": true,
		"rollout": true, "scale": true, "edit": true, "set": true, "annotate": true, "label": true,
	},
	"helm":     {"install": true, "upgrade": true, "uninstall": true, "rollback": true},
	"gcloud":   {"deploy": true},
	"aws":      {"deploy": true},
	"pulumi":   {"up": true, "destroy": true},
	"docker":   {"push": true},
	"firebase": {"deploy": true},
	"sam":      {"deploy": true},
	"gh":       {"release": true},
	"cargo":    {"publish": true},
	"poetry":   {"publish": true},
	"gem":      {"push": true},
	"twine":    {"upload": true},
}

// runnerTools are tools that run a named task, where a deploy verb names the task.
var runnerTools = map[string]bool{
	"npm": true, "pnpm": true, "yarn": true, "bun": true, "make": true, "just": true, "task": true,
}

// deployVerbsEverywhere are the task names that are a deploy whatever the runner.
var deployVerbsEverywhere = map[string]bool{
	"deploy": true, "release": true, "publish": true, "promote": true, "rollout": true,
}

// skipRunner drops the word a runner uses to introduce a script ("npm run deploy", "yarn run
// release"), so the task name is the word that is compared.
func skipRunner(rest []string) []string {
	for i, a := range rest {
		if a == "run" || a == "--" {
			return rest[i+1:]
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return rest[i:]
	}
	return nil
}

// anyVerb says whether any word is one of the given verbs.
func anyVerb(words []string, verbs map[string]bool) bool {
	for _, a := range words {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if verbs[a] {
			return true
		}
	}
	return false
}
