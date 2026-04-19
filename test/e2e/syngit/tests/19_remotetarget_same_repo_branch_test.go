/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package tests

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	syngiterrors "github.com/syngit-org/syngit/pkg/errors"
	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("19 RemoteTarget same repo & branch between target and upstream", func() {

	const (
		repo            = "https://my-server.com/my-upstream-repo.git"
		differentRepo   = "https://my-server.com/my-different-repo.git"
		branch          = "main"
		differentBranch = "different"
	)

	It("denies the RemoteTarget when upstream == target with a non-empty merge strategy", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		rt := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{Name: "remotetarget-test19-same", Namespace: fx.Namespace},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: repo,
				TargetRepository:   repo,
				UpstreamBranch:     branch,
				TargetBranch:       branch,
				MergeStrategy:      syngit.TryFastForwardOrHardReset,
			},
		}
		Eventually(func() bool {
			err := fx.Users.CreateOrUpdate(ctx, utils.Developer, rt)
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrWrongRemoteSyncerConfig) // nolint:lll
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})

	It("denies the RemoteTarget when upstream != target with an empty merge strategy", func() {
		ctx := context.Background()
		fx := suite.NewFixture(ctx)

		rt := &syngit.RemoteTarget{
			ObjectMeta: metav1.ObjectMeta{Name: "remotetarget-test19-diff", Namespace: fx.Namespace},
			Spec: syngit.RemoteTargetSpec{
				UpstreamRepository: repo,
				TargetRepository:   differentRepo,
				UpstreamBranch:     branch,
				TargetBranch:       differentBranch,
				MergeStrategy:      "",
			},
		}
		Eventually(func() bool {
			err := fx.Users.CreateOrUpdate(ctx, utils.Developer, rt)
			return err != nil && syngiterrors.Is(err, syngiterrors.ErrWrongRemoteSyncerConfig) // nolint:lll
		}).WithTimeout(utils.DefaultTimeout).WithPolling(utils.DefaultInterval).Should(BeTrue())
	})
})
