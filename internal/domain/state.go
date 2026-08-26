package domain

import "fmt"

type AssayState string

const (
	StateDraft      AssayState = "draft"
	StateFrozen     AssayState = "frozen"
	StateObserving  AssayState = "observing"
	StateSealed     AssayState = "sealed"
	StateCorrection AssayState = "correction"
	StateReview     AssayState = "review"
	StateApproved   AssayState = "approved"
	StateArchived   AssayState = "archived"
)

var transitions = map[AssayState]map[AssayState]bool{
	StateDraft:      {StateFrozen: true},
	StateFrozen:     {StateObserving: true},
	StateObserving:  {StateSealed: true},
	StateSealed:     {StateReview: true},
	StateReview:     {StateCorrection: true, StateApproved: true},
	StateCorrection: {StateReview: true},
	StateApproved:   {StateArchived: true},
}

func (s AssayState) CanTransition(next AssayState) bool {
	return transitions[s][next]
}

func (s AssayState) Validate() error {
	switch s {
	case StateDraft, StateFrozen, StateObserving, StateSealed, StateCorrection, StateReview, StateApproved, StateArchived:
		return nil
	default:
		return fmt.Errorf("未知批次状态 %q", s)
	}
}

func (s AssayState) IsImmutable() bool { return s == StateArchived }
