/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"syngit.io/syngit/api/v1beta1"
)

func (src *RemoteUser) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.RemoteUser)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Email = src.Spec.Email
	gitBaseDomainFQDN := src.Spec.GitBaseDomainFQDN
	dst.Spec.GitBaseDomainFQDN = gitBaseDomainFQDN
	dst.Spec.SecretRef = src.Spec.SecretRef

	dst.Status.Conditions = src.Status.Conditions
	dst.Status.ConnexionStatus.Details = src.Status.ConnexionStatus.Details
	dst.Status.ConnexionStatus.Status = v1beta1.RemoteUserConnexionStatusReason(src.Status.ConnexionStatus.Status)
	dst.Status.GitUser = src.Status.GitUser
	dst.Status.LastAuthTime = src.Status.LastAuthTime
	dst.Status.SecretBoundStatus = v1beta1.SecretBoundStatus(src.Status.SecretBoundStatus)

	// Breaking changes
	if gitBaseDomainFQDN == "github.com" {
		insecureSkipTlsVerify := src.Spec.InsecureSkipTlsVerify
		dst.Annotations["syngit.syngit.io/github.api.auth.insecure-skip-tls-verify"] = strconv.FormatBool(insecureSkipTlsVerify)

		testAuthentication := src.Spec.TestAuthentication
		dst.Annotations["syngit.syngit.io/github.api-auth.test"] = strconv.FormatBool(testAuthentication)
	}
	if gitBaseDomainFQDN == "gitlab.com" {
		insecureSkipTlsVerify := src.Spec.InsecureSkipTlsVerify
		dst.Annotations["syngit.syngit.io/gitlab.api.auth.insecure-skip-tls-verify"] = strconv.FormatBool(insecureSkipTlsVerify)

		testAuthentication := src.Spec.TestAuthentication
		dst.Annotations["syngit.syngit.io/gitlab.api-auth.test"] = strconv.FormatBool(testAuthentication)
	}
	if gitBaseDomainFQDN == "bitbucket.org" {
		insecureSkipTlsVerify := src.Spec.InsecureSkipTlsVerify
		dst.Annotations["syngit.syngit.io/bitbucket.api-auth.insecure-skip-tls-verify"] = strconv.FormatBool(insecureSkipTlsVerify)

		testAuthentication := src.Spec.TestAuthentication
		dst.Annotations["syngit.syngit.io/bitbucket.api-auth.test"] = strconv.FormatBool(testAuthentication)
	}

	return nil
}

func (dst *RemoteUser) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.RemoteUser)

	// Common conversion
	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Email = src.Spec.Email
	gitBaseDomainFQDN := src.Spec.GitBaseDomainFQDN
	dst.Spec.GitBaseDomainFQDN = gitBaseDomainFQDN
	dst.Spec.SecretRef = src.Spec.SecretRef

	dst.Status.Conditions = src.Status.Conditions
	dst.Status.ConnexionStatus.Details = src.Status.ConnexionStatus.Details
	dst.Status.ConnexionStatus.Status = RemoteUserConnexionStatusReason(src.Status.ConnexionStatus.Status)
	dst.Status.GitUser = src.Status.GitUser
	dst.Status.LastAuthTime = src.Status.LastAuthTime
	dst.Status.SecretBoundStatus = SecretBoundStatus(src.Status.SecretBoundStatus)

	// Breaking changes
	if gitBaseDomainFQDN == "github.com" {
		insecureSkipTlsAnnotation := src.Annotations["syngit.syngit.io/github.api-auth.insecure-skip-tls-verify"]
		if insecureSkipTlsAnnotation != "" {
			insecureSkipTlsVerify, err := strconv.ParseBool(insecureSkipTlsAnnotation)
			dst.Spec.InsecureSkipTlsVerify = insecureSkipTlsVerify
			if err != nil {
				return err
			}
		}
		testAuthAnnotation := src.Annotations["syngit.syngit.io/github.api-auth.test"]
		if testAuthAnnotation != "" {
			testAuthentication, err := strconv.ParseBool(testAuthAnnotation)
			dst.Spec.TestAuthentication = testAuthentication
			if err != nil {
				return err
			}
		}
	}
	if gitBaseDomainFQDN == "gitlab.com" {
		insecureSkipTlsAnnotation := src.Annotations["syngit.syngit.io/github.api-auth.insecure-skip-tls-verify"]
		if insecureSkipTlsAnnotation != "" {
			insecureSkipTlsVerify, err := strconv.ParseBool(src.Annotations["syngit.syngit.io/gitlab.api-auth.insecure-skip-tls-verify"])
			dst.Spec.InsecureSkipTlsVerify = insecureSkipTlsVerify
			if err != nil {
				return err
			}
		}
		testAuthAnnotation := src.Annotations["syngit.syngit.io/github.api-auth.test"]
		if testAuthAnnotation != "" {
			testAuthentication, err := strconv.ParseBool(src.Annotations["syngit.syngit.io/gitlab.api-auth.test"])
			dst.Spec.TestAuthentication = testAuthentication
			if err != nil {
				return err
			}
		}
	}
	if gitBaseDomainFQDN == "bitbucket.org" {
		insecureSkipTlsAnnotation := src.Annotations["syngit.syngit.io/github.api-auth.insecure-skip-tls-verify"]
		if insecureSkipTlsAnnotation != "" {
			insecureSkipTlsVerify, err := strconv.ParseBool(src.Annotations["syngit.syngit.io/bitbucket.api-auth.insecure-skip-tls-verify"])
			dst.Spec.InsecureSkipTlsVerify = insecureSkipTlsVerify
			if err != nil {
				return err
			}
		}
		testAuthAnnotation := src.Annotations["syngit.syngit.io/github.api-auth.test"]
		if testAuthAnnotation != "" {
			testAuthentication, err := strconv.ParseBool(src.Annotations["syngit.syngit.io/bitbucket.api-auth.test"])
			dst.Spec.TestAuthentication = testAuthentication
			if err != nil {
				return err
			}
		}
	}

	return nil
}
