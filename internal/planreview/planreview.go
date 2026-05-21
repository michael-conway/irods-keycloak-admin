package planreview

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

type PromptMode string

const (
	PromptModeRequired PromptMode = "required"
	PromptModeAll      PromptMode = "all"
	PromptModeNone     PromptMode = "none"
)

type Decision string

const (
	DecisionAccept    Decision = "accept"
	DecisionSkip      Decision = "skip"
	DecisionAcceptAll Decision = "accept_all"
	DecisionSkipAll   Decision = "skip_all"
)

type Reviewer interface {
	Review(ctx context.Context, plan domain.SyncPlan, operation domain.PlanOperation) (Decision, error)
}

type Session struct {
	mode    PromptMode
	review  Reviewer
	skipAll bool
}

type Result struct {
	Plan     domain.SyncPlan
	Accepted int
	Skipped  int
}

func NewSession(mode PromptMode, reviewer Reviewer) (*Session, error) {
	if mode == "" {
		mode = PromptModeRequired
	}
	if err := ValidatePromptMode(mode); err != nil {
		return nil, err
	}
	return &Session{mode: mode, review: reviewer}, nil
}

func ValidatePromptMode(mode PromptMode) error {
	switch mode {
	case PromptModeRequired, PromptModeAll, PromptModeNone:
		return nil
	default:
		return fmt.Errorf("unsupported prompts mode %q; expected none, required, or all", mode)
	}
}

func (s *Session) Decide(ctx context.Context, plan domain.SyncPlan, operation domain.PlanOperation) (Decision, error) {
	if s == nil {
		return "", errors.New("plan review session is required")
	}
	if s.skipAll {
		return DecisionSkip, nil
	}
	if !s.shouldPrompt(operation) {
		return DecisionAccept, nil
	}
	if s.review == nil {
		return "", fmt.Errorf("operation %q requires a prompt, but no reviewer is configured", operation.OperationID)
	}
	decision, err := s.review.Review(ctx, plan, operation)
	if err != nil {
		return "", err
	}
	decision, err = NormalizeDecision(decision)
	if err != nil {
		return "", err
	}
	switch decision {
	case DecisionAcceptAll:
		s.mode = PromptModeNone
		return DecisionAccept, nil
	case DecisionSkipAll:
		s.skipAll = true
		return DecisionSkip, nil
	default:
		return decision, nil
	}
}

func (s *Session) shouldPrompt(operation domain.PlanOperation) bool {
	switch s.mode {
	case PromptModeNone:
		return false
	case PromptModeAll:
		return true
	default:
		return operation.Risk == domain.PlanRiskRequiresApproval
	}
}

func Review(ctx context.Context, plan domain.SyncPlan, mode PromptMode, reviewer Reviewer) (Result, error) {
	session, err := NewSession(mode, reviewer)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Plan: plan,
	}
	result.Plan.Operations = []domain.PlanOperation{}
	for _, operation := range plan.Operations {
		decision, err := session.Decide(ctx, plan, operation)
		if err != nil {
			return Result{}, err
		}
		switch decision {
		case DecisionAccept:
			result.Plan.Operations = append(result.Plan.Operations, operation)
			result.Accepted++
		case DecisionSkip:
			result.Skipped++
		default:
			return Result{}, fmt.Errorf("unsupported normalized decision %q", decision)
		}
	}
	return result, nil
}

func NormalizeDecision(decision Decision) (Decision, error) {
	value := strings.ToLower(strings.TrimSpace(string(decision)))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	switch value {
	case "a", "accept":
		return DecisionAccept, nil
	case "s", "skip":
		return DecisionSkip, nil
	case "aa", "all", "accept_all", "acceptall":
		return DecisionAcceptAll, nil
	case "ss", "sa", "skip_all", "skipall":
		return DecisionSkipAll, nil
	default:
		return "", fmt.Errorf("unsupported review decision %q", decision)
	}
}
