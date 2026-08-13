package modules

import (
	"vraxel.io/vraxel/lib/ansible/modules/add_hostvars"
	"vraxel.io/vraxel/lib/ansible/modules/assert"
	"vraxel.io/vraxel/lib/ansible/modules/command"
	mcopy "vraxel.io/vraxel/lib/ansible/modules/copy"
	"vraxel.io/vraxel/lib/ansible/modules/debug"
	"vraxel.io/vraxel/lib/ansible/modules/fetch"
	mfile "vraxel.io/vraxel/lib/ansible/modules/file"
	"vraxel.io/vraxel/lib/ansible/modules/http_get_file"
	"vraxel.io/vraxel/lib/ansible/modules/include_vars"
	"vraxel.io/vraxel/lib/ansible/modules/internal"
	"vraxel.io/vraxel/lib/ansible/modules/result"
	"vraxel.io/vraxel/lib/ansible/modules/service"
	"vraxel.io/vraxel/lib/ansible/modules/set_fact"
	"vraxel.io/vraxel/lib/ansible/modules/setup"
	"vraxel.io/vraxel/lib/ansible/modules/stat"
	"vraxel.io/vraxel/lib/ansible/modules/template"
	"vraxel.io/vraxel/lib/ansible/modules/wait_for"
)

// Re-export types from internal so external packages can use them
// via the modules package without importing internal directly.
type ExecOptions = internal.ExecOptions
type ModuleExecFunc = internal.ModuleExecFunc
type Source = internal.Source

// Re-export registry functions.
var (
	RegisterModule = internal.RegisterModule
	FindModule     = internal.FindModule
	IsModule       = internal.IsModule
	ListModules    = internal.ListModules
)

// Re-export helper functions for top-level test access.
var (
	StringArg   = internal.StringArg
	FileModeArg = internal.FileModeArg
	ReadSource  = internal.ReadSource
)

func init() {
	// Register all modules centrally.
	internal.RegisterModule("add_hostvars", add_hostvars.ModuleAddHostvars)
	internal.RegisterModule("assert", assert.ModuleAssert)
	internal.RegisterModule("command", command.ModuleCommand)
	internal.RegisterModule("shell", command.ModuleShell)
	internal.RegisterModule("copy", mcopy.ModuleCopy)
	internal.RegisterModule("debug", debug.ModuleDebug)
	internal.RegisterModule("fetch", fetch.ModuleFetch)
	internal.RegisterModule("file", mfile.ModuleFile)
	internal.RegisterModule("http_get_file", http_get_file.ModuleHTTPGetFile)
	internal.RegisterModule("include_vars", include_vars.ModuleIncludeVars)
	internal.RegisterModule("result", result.ModuleResult)
	// service and systemd are the same implementation: this engine targets
	// systemd hosts, so the generic name has no second backend to pick.
	internal.RegisterModule("service", service.ModuleService)
	internal.RegisterModule("systemd", service.ModuleService)
	internal.RegisterModule("set_fact", set_fact.ModuleSetFact)
	internal.RegisterModule("setup", setup.ModuleSetup)
	internal.RegisterModule("stat", stat.ModuleStat)
	internal.RegisterModule("template", template.ModuleTemplate)
	internal.RegisterModule("wait_for", wait_for.ModuleWaitFor)
}
