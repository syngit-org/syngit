package v1beta3

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PatternSpecification struct {
	Client         client.Client
	NamespacedName types.NamespacedName
}

type Pattern interface {
	Setup(ctx context.Context) *errorPattern
	Diff(ctx context.Context) *errorPattern
	Remove(ctx context.Context) *errorPattern
}

type reason string

const (
	Errored reason = "Errored"
	Denied  reason = "Denied"
)

type errorPattern struct {
	Message string
	Reason  reason
}

func (e *errorPattern) Error() string {
	return e.Message
}

func Trigger(p Pattern, ctx context.Context) *errorPattern {

	diffErr := p.Diff(ctx)
	if diffErr != nil {
		return diffErr
	}
	removeErr := p.Remove(ctx)
	if removeErr != nil {
		return removeErr
	}
	setupErr := p.Setup(ctx)
	if setupErr != nil {
		return setupErr
	}

	return nil
}
