package ansible

import (
	"strings"
	"testing"
)

// taskWith builds a minimal block carrying a module plus whatever the case
// under test sets.
func taskWith(set func(*Block)) Block {
	b := Block{}
	b.Name = "t"
	b.UnknownField = map[string]any{"shell": "true"}
	set(&b)
	return b
}

func TestValidateBlocks_RejectsUnimplementedDirectives(t *testing.T) {
	cases := map[string]struct {
		set  func(*Block)
		want string
	}{
		"notify":           {func(b *Block) { b.Notify = "restart" }, "notify"},
		"delegate_facts":   {func(b *Block) { b.DelegateFacts = true }, "delegate_facts"},
		"async":            {func(b *Block) { b.AsyncVal = 30 }, "async"},
		"poll":             {func(b *Block) { b.Poll = 5 }, "async"},
		"any_errors_fatal": {func(b *Block) { b.AnyErrorsFatal = true }, "any_errors_fatal"},
		"throttle":         {func(b *Block) { b.Throttle = 2 }, "throttle"},
		"timeout":          {func(b *Block) { b.Timeout = 30 }, "timeout"},
		"debugger":         {func(b *Block) { b.Debugger = "on_failed" }, "debugger"},
		"with_fileglob":    {func(b *Block) { b.UnknownField["with_fileglob"] = "/etc/*" }, "with_fileglob"},
		"with_together":    {func(b *Block) { b.UnknownField["with_together"] = "x" }, "with_together"},
		"include_role":     {func(b *Block) { b.UnknownField["include_role"] = "x" }, "include_role"},
		"import_role":      {func(b *Block) { b.UnknownField["import_role"] = "x" }, "import_role"},
		"local_action":     {func(b *Block) { b.UnknownField["local_action"] = "x" }, "local_action"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateBlocks([]Block{taskWith(tc.set)})
			if err == nil {
				t.Fatalf("expected %s to be rejected, got nil", name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error should name %q, got %q", tc.want, err.Error())
			}
			if !strings.Contains(err.Error(), `task "t"`) {
				t.Errorf("error should locate the task, got %q", err.Error())
			}
		})
	}
}

func TestValidateBlocks_AcceptsImplementedDirectives(t *testing.T) {
	// The point of a load-time gate is that it rejects only what the engine
	// really cannot do. A false positive here breaks working playbooks.
	cases := map[string]func(*Block){
		"loop":          func(b *Block) { b.Loop = []any{"a"} },
		"with_items":    func(b *Block) { b.WithItems = "{{ .l }}" },
		"with_dict":     func(b *Block) { b.WithDict = "{{ .m }}" },
		"loop_control":  func(b *Block) { b.LoopControl = LoopControl{LoopVar: "x", IndexVar: "i"} },
		"delegate_to":   func(b *Block) { b.DelegateTo = "localhost" },
		"changed_when":  func(b *Block) { b.ChangedWhen = When{Data: []string{"{{ false }}"}} },
		"failed_when":   func(b *Block) { b.FailedWhen = When{Data: []string{"{{ false }}"}} },
		"environment":   func(b *Block) { b.Environment = map[string]any{"A": "b"} },
		"run_once":      func(b *Block) { b.RunOnce = true },
		"no_log":        func(b *Block) { b.NoLog = true },
		"register":      func(b *Block) { b.Register = "r" },
		"retries":       func(b *Block) { b.Retries = 3 },
		"become":        func(b *Block) { b.Become = true },
		"ignore_errors": func(b *Block) { yes := true; b.IgnoreErrors = &yes },
		// check_mode / diff are inert rather than misleading: the engine has
		// no dry-run mode, so rejecting them would only break playbooks that
		// set them defensively.
		"check_mode": func(b *Block) { b.CheckMode = true },
		"diff":       func(b *Block) { b.Diff = true },
	}

	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateBlocks([]Block{taskWith(set)}); err != nil {
				t.Errorf("%s must stay accepted, got %v", name, err)
			}
		})
	}
}

func TestValidateBlocks_RejectsConflictingLoopDirectives(t *testing.T) {
	err := ValidateBlocks([]Block{taskWith(func(b *Block) {
		b.Loop = []any{"a"}
		b.WithItems = "{{ .l }}"
	})})
	if err == nil {
		t.Fatal("expected two loop directives on one task to be rejected")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should say mutually exclusive, got %q", err.Error())
	}
}

func TestValidateBlocks_RejectsBlockLevelEnvironment(t *testing.T) {
	// A container's environment is not inherited by its tasks, so accepting
	// it would be a silent no-op.
	container := Block{}
	container.Name = "c"
	container.Environment = map[string]any{"A": "b"}
	container.BlockInfo.Block = []Block{taskWith(func(*Block) {})}

	err := ValidateBlocks([]Block{container})
	if err == nil {
		t.Fatal("expected block-level environment to be rejected")
	}
	if !strings.Contains(err.Error(), "block-level environment") {
		t.Errorf("error should name the directive, got %q", err.Error())
	}
}

func TestValidatePlay_RejectsPlayLevelEnvironment(t *testing.T) {
	play := Play{PlayHost: PlayHost{Hosts: []string{"all"}}}
	play.Environment = map[string]any{"A": "b"}

	err := ValidatePlay(play)
	if err == nil {
		t.Fatal("expected play-level environment to be rejected")
	}
	if !strings.Contains(err.Error(), "play-level environment") {
		t.Errorf("error should name the directive, got %q", err.Error())
	}
}

func TestValidateBlocks_DescendsIntoBlockRescueAlways(t *testing.T) {
	for name, build := range map[string]func(Block) Block{
		"block":  func(inner Block) Block { b := Block{}; b.BlockInfo.Block = []Block{inner}; return b },
		"rescue": func(inner Block) Block { b := Block{}; b.BlockInfo.Rescue = []Block{inner}; return b },
		"always": func(inner Block) Block { b := Block{}; b.BlockInfo.Always = []Block{inner}; return b },
	} {
		t.Run(name, func(t *testing.T) {
			inner := taskWith(func(b *Block) { b.Notify = "restart" })
			if err := ValidateBlocks([]Block{build(inner)}); err == nil {
				t.Errorf("expected the nested %s task to be rejected", name)
			}
		})
	}
}

func TestValidatePlay_RejectsUnimplementedDirectives(t *testing.T) {
	cases := map[string]struct {
		set  func(*Play)
		want string
	}{
		"strategy":            {func(p *Play) { p.Strategy = "free" }, "strategy"},
		"order":               {func(p *Play) { p.Order = "reverse_sorted" }, "order"},
		"force_handlers":      {func(p *Play) { p.ForceHandlers = true }, "force_handlers"},
		"max_fail_percentage": {func(p *Play) { p.MaxFailPercentage = 30 }, "max_fail_percentage"},
		"handlers":            {func(p *Play) { p.Handlers = []Block{{}} }, "handlers"},
		"any_errors_fatal":    {func(p *Play) { p.AnyErrorsFatal = true }, "any_errors_fatal"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			play := Play{PlayHost: PlayHost{Hosts: []string{"all"}}}
			play.Name = "p"
			tc.set(&play)

			err := ValidatePlay(play)
			if err == nil {
				t.Fatalf("expected %s to be rejected, got nil", name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error should name %q, got %q", tc.want, err.Error())
			}
		})
	}
}

func TestValidatePlay_AcceptsImplementedDirectives(t *testing.T) {
	play := Play{
		PlayHost:    PlayHost{Hosts: []string{"all"}},
		GatherFacts: true,
		Serial:      PlaySerial{Data: []any{1}},
	}
	play.Name = "p"
	play.Become = true
	play.Strategy = "linear" // the strategy the engine actually implements

	if err := ValidatePlay(play); err != nil {
		t.Errorf("a plain play must stay accepted, got %v", err)
	}
}

func TestPlaybookValidate_ChecksTaskLists(t *testing.T) {
	for name, build := range map[string]func(Block) Play{
		"pre_tasks":  func(b Block) Play { return Play{PlayHost: PlayHost{Hosts: []string{"all"}}, PreTasks: []Block{b}} },
		"tasks":      func(b Block) Play { return Play{PlayHost: PlayHost{Hosts: []string{"all"}}, Tasks: []Block{b}} },
		"post_tasks": func(b Block) Play { return Play{PlayHost: PlayHost{Hosts: []string{"all"}}, PostTasks: []Block{b}} },
	} {
		t.Run(name, func(t *testing.T) {
			pb := &Playbook{Play: []Play{build(taskWith(func(b *Block) { b.Notify = "restart" }))}}
			if err := pb.Validate(); err == nil {
				t.Errorf("expected the %s entry to be rejected", name)
			}
		})
	}
}
