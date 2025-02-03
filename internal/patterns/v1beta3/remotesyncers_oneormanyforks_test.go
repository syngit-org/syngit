package v1beta3

import "context"

type RemoteSyncerOneOrManyForkPattern struct {
	PatternSpecification
	TargetForks []string
}

func (rsomp *RemoteSyncerOneOrManyForkPattern) Trigger(ctx context.Context) *errorPattern {

	return nil
}

func (rsomp *RemoteSyncerOneOrManyForkPattern) RemoveExistingOnes(ctx context.Context) error {
	return nil
}
