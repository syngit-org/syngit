/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package tests

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	utils "github.com/syngit-org/syngit/test/e2e/syngit/utils"
)

// suite is the shared bootstrap owned by BeforeSuite and torn down by
// AfterSuite. Every spec accesses it through suite.NewFixture(ctx).
var suite *utils.Suite

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "syngit end-to-end suite")
}

var _ = BeforeSuite(func() {
	suite = utils.Bootstrap()
})

var _ = AfterSuite(func() {
	suite.Teardown()
})
