// Package planner computes the orchestration plan of a playbook — play
// ordering, host resolution (group names and "group[0]" index syntax),
// serial batching and run_once target selection — without executing
// anything. The executor consumes this plan for SSH-mode execution, and
// the server side will consume it to split a playbook into per-(play,
// host) agent jobs. It must therefore stay free of execution
// dependencies (connectors, modules).
//
// Planning only reads inventory-derived state: nothing in the engine
// adds or removes hosts at runtime (add_hostvars only mutates vars of
// existing hosts), so a plan computed up front is exactly what the
// executor would have resolved play by play.
package planner

import (
	"fmt"

	"vraxel.io/vraxel/lib/ansible"
	"vraxel.io/vraxel/lib/ansible/converter"
	"vraxel.io/vraxel/lib/ansible/variable"
)

// PlayPlan describes one (play, batch) execution unit. Batches of a play
// are contiguous slices of its resolved host list, so concatenating a
// play's batches in BatchIndex order reproduces that list exactly.
type PlayPlan struct {
	PlayIndex  int
	BatchIndex int
	Hosts      []string
	// RunOnceTarget is the host a run_once task executes on within this
	// batch (the batch's first host); empty only for an empty batch.
	// Whether run_once actually applies is a block-level property
	// resolved at execution time (executor/block.go) — blocks inside
	// roles and include_tasks files are not visible at planning time —
	// so the planner always designates the batch leader.
	RunOnceTarget string
}

// Planner resolves play host patterns against a fixed inventory.
type Planner struct {
	v variable.Variable
}

// New creates a Planner for the given inventory.
func New(inv ansible.Inventory) *Planner {
	return &Planner{v: variable.New(inv)}
}

// PlanPlay computes the ordered batches for a single play. A play
// matching no hosts (empty or unknown group, out-of-range index) yields
// no entries — standard ansible semantics: the play is skipped, not a
// fatal error. Its serial spec is not validated in that case, matching
// the executor's original early return. A serial spec listing more
// entries than there are hosts produces trailing empty batches, also
// preserved from GroupHostBySerial.
func (p *Planner) PlanPlay(playIndex int, play ansible.Play) ([]PlayPlan, error) {
	hosts, _ := p.v.Get(variable.GetHostnames(play.PlayHost.Hosts)).([]string)
	if len(hosts) == 0 {
		return nil, nil
	}

	batches, err := converter.GroupHostBySerial(hosts, play.Serial.Data)
	if err != nil {
		return nil, fmt.Errorf("serial batching: %w", err)
	}

	plan := make([]PlayPlan, 0, len(batches))
	for i, batch := range batches {
		pp := PlayPlan{PlayIndex: playIndex, BatchIndex: i, Hosts: batch}
		if len(batch) > 0 {
			pp.RunOnceTarget = batch[0]
		}
		plan = append(plan, pp)
	}
	return plan, nil
}

// BuildPlayPlan computes the full execution plan for a list of plays.
func BuildPlayPlan(plays []ansible.Play, inv ansible.Inventory) ([]PlayPlan, error) {
	p := New(inv)
	var plan []PlayPlan
	for i, play := range plays {
		pp, err := p.PlanPlay(i, play)
		if err != nil {
			return nil, fmt.Errorf("play %d '%s': %w", i+1, play.Name, err)
		}
		plan = append(plan, pp...)
	}
	return plan, nil
}
