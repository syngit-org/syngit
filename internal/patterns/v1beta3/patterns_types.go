package v1beta3

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PatternSpecification struct {
	Client         client.Client
	NamespacedName types.NamespacedName
	Username       string
}

type Pattern interface {
	Trigger(ctx context.Context) *errorPattern
	RemoveExistingOnes(ctx context.Context) error
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
