package utils

import (
	"fmt"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta3"
	authv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
)

// REMOTE SYNCER WEBHOOK

/*
	This error must be used when the user is not allowed to
	select a set of resources for interception.
*/

type ResourceScopeForbiddenError struct {
	User               authv1.UserInfo
	ForbiddenResources []string
}

func (rsfe ResourceScopeForbiddenError) Error() string {
	return fmt.Sprintf("The user %s is not allowed to scope: \n- %s",
		rsfe.User,
		strings.Join(rsfe.ForbiddenResources, "\n- "),
	)
}

func (rsfe ResourceScopeForbiddenError) ShouldContains(errorString string) bool {
	for _, str := range []string{
		"The user ",
		" is not allowed to scope: ",
	} {
		if !strings.Contains(errorString, str) {
			return false
		}
	}
	return true
}

// REMOTE USER PATTERN ASSOCIATION WEBHOOK

/*
	This error must be used when the RemoteUser that
	suppose to be associated with a RemoteUserBinding
	using the pattern is already associated with another one.
*/

type RemoteUserAlreadyBoundError struct {
	ExistingRemoteUserBindingName string
}

func (ruabe RemoteUserAlreadyBoundError) Error() string {
	return fmt.Sprintf("the RemoteUser is already bound in the RemoteUserBinding %s",
		ruabe.ExistingRemoteUserBindingName,
	)
}

func (ruabe RemoteUserAlreadyBoundError) ShouldContains(errorString string) bool {
	for _, str := range []string{
		"the RemoteUser is already bound in the RemoteUserBinding ",
	} {
		if !strings.Contains(errorString, str) {
			return false
		}
	}
	return true
}

/*
	This interface must be used by the errors that
	are raised during the interception process.

	Terminology used in this file:
	"user": the user that trigger an interception
					by creating, updating or deleting a resource
*/

type ResourceInterceptorError interface {
	Error() string
	ShouldContains(errorString string) bool
}

func ErrorTypeChecker(errorType ResourceInterceptorError, errorString string) bool {
	return errorType.ShouldContains(errorString)
}

// ---

/*
	This error must be used when the user is not allowed
	to get the secret referenced by its own RemoteUser.
*/

type DenyGetSecretError struct {
	User      authv1.UserInfo
	SecretRef v1.SecretReference
}

func (gse DenyGetSecretError) Error() string {
	return fmt.Sprintf("The user %s is not allowed to get the secret: %s",
		gse.User,
		gse.SecretRef.Name,
	)
}

func (gse DenyGetSecretError) ShouldContains(errorString string) bool {
	for _, str := range []string{
		"The user ",
		" is not allowed to get the secret: ",
	} {
		if !strings.Contains(errorString, str) {
			return false
		}
	}
	return true
}

// ---

/*
	This error must be used when the user is not allowed
	to get its own RemoteUser.
*/

type DenyGetRemoteUserError struct {
	User          authv1.UserInfo
	RemoteUserRef v1.ObjectReference
}

func (grue DenyGetRemoteUserError) Error() string {
	return fmt.Sprintf("The user %s is not allowed to get the referenced remoteuser: %s",
		grue.User,
		grue.RemoteUserRef.Name,
	)
}

func (grue DenyGetRemoteUserError) ShouldContains(errorString string) bool {
	for _, str := range []string{
		"The user ",
		" is not allowed to get the referenced remoteuser: ",
	} {
		if !strings.Contains(errorString, str) {
			return false
		}
	}
	return true
}

/*
	This error must be used when there is no RemoteUserBinding
	referencing the user as the subject.
*/

type RemoteUserBindingNotFoundError struct {
	Username string
}

func (rubnfe RemoteUserBindingNotFoundError) Error() string {
	return fmt.Sprintf("no RemoteUserBinding found for the user %s",
		rubnfe.Username,
	)
}

func (rubnfe RemoteUserBindingNotFoundError) ShouldContains(errorString string) bool {
	for _, str := range []string{
		"no RemoteUserBinding found for the user ",
	} {
		if !strings.Contains(errorString, str) {
			return false
		}
	}
	return true
}

/*
	This error must be used when the defaultRemoteUser
	referenced in the RemoteSyncer does not exists.
*/

type DefaultRemoteUserNotFoundError struct {
	DefaultUserName string
}

func (drunfe DefaultRemoteUserNotFoundError) Error() string {
	return fmt.Sprintf("the default RemoteUser is not found: %s",
		drunfe.DefaultUserName,
	)
}

func (drunfe DefaultRemoteUserNotFoundError) ShouldContains(errorString string) bool {
	for _, str := range []string{
		"the default RemoteUser is not found: ",
	} {
		if !strings.Contains(errorString, str) {
			return false
		}
	}
	return true
}

/*
	This error must be used when the defaultRemoteTarget
	referenced in the RemoteSyncer does not exists.
*/

type DefaultRemoteTargetNotFoundError struct {
	DefaultTargetName string
}

func (drtnfe DefaultRemoteTargetNotFoundError) Error() string {
	return fmt.Sprintf("the default RemoteTarget is not found: %s",
		drtnfe.DefaultTargetName,
	)
}

func (drtnfe DefaultRemoteTargetNotFoundError) ShouldContains(errorString string) bool {
	for _, str := range []string{
		"the default RemoteTarget is not found: ",
	} {
		if !strings.Contains(errorString, str) {
			return false
		}
	}
	return true
}

/*
	This error must be used when the defaultRemoteUser's fqdn
	does not match the RemoteSyncer's fqdn.
*/

type DefaultRemoteTargetMismatchError struct {
	RemoteSyncer syngit.RemoteSyncer
	RemoteUser   syngit.RemoteUser
}

func (drtme DefaultRemoteTargetMismatchError) Error() string {
	return fmt.Sprintf("the fqdn of the default RemoteUser does not match the associated RemoteSyncer (%s) fqdn (%s)",
		drtme.RemoteSyncer.Name,
		drtme.RemoteUser.Name,
	)
}

// ---

type SameUpstreamDifferentMergeStrategyError struct {
	UpstreamRepository string
	UpstreamBranch     string
	TargetRepository   string
	TargetBranch       string
	MergeStrategy      string
}

func (sudmse SameUpstreamDifferentMergeStrategyError) Error() string {
	return "should not be set when the target repo & target branch are the same as the upstream repo & branch"
}

func (sudmse SameUpstreamDifferentMergeStrategyError) ShouldContains(errorString string) bool {
	for _, str := range []string{
		sudmse.Error(),
	} {
		if !strings.Contains(errorString, str) {
			return false
		}
	}
	return true
}

// ---

type DifferentUpstreamEmptyMergeStrategyError struct {
	UpstreamRepository string
	UpstreamBranch     string
	TargetRepository   string
	TargetBranch       string
	MergeStrategy      string
}

func (duemse DifferentUpstreamEmptyMergeStrategyError) Error() string {
	return "should be set when the target repo & target branch are different from the upstream repo & branch"
}

func (duemse DifferentUpstreamEmptyMergeStrategyError) ShouldContains(errorString string) bool {
	for _, str := range []string{
		duemse.Error(),
	} {
		if !strings.Contains(errorString, str) {
			return false
		}
	}
	return true
}

/*
	This error must be used when there is not RemoteTarget
	corresponding to these requirements:
	- remoteTargetLabelSelector
	- upstream repo = remotesyncer repository
	- upstream branch = remotesyncer default branch
*/

type RemoteTargetNotFoundError struct{}

func (rtnfe RemoteTargetNotFoundError) Error() string {
	return "no RemoteTarget found"
}

func (rtnfe RemoteTargetNotFoundError) ShouldContains(errorString string) bool {
	for _, str := range []string{
		rtnfe.Error(),
	} {
		if !strings.Contains(errorString, str) {
			return false
		}
	}
	return true
}

/*
	This error must be used when there is no RemoteUser
	corresponding to these requirements:
	- remoteuser fqdn = remotesyncer fqdn
	- only one remoteuser found for this user for this platform
*/

type RemoteUserSearchErrorReason string

const (
	RemoteUserNotFound         RemoteUserSearchErrorReason = "no RemoteUser found for the current user with this fqdn: "
	MoreThanOneRemoteUserFound RemoteUserSearchErrorReason = "more than one RemoteUser found for the current user with this fqdn: " //nolint:lll
)

type RemoteUserSearchError struct {
	Reason RemoteUserSearchErrorReason
	Fqdn   string
}

func (ruse RemoteUserSearchError) Error() string {
	return fmt.Sprintf("%s%s", ruse.Reason, ruse.Fqdn)
}

func (ruse RemoteUserSearchError) ShouldContains(errorString string) bool {
	if strings.Contains(errorString, string(RemoteUserNotFound)) {
		return true
	}
	if strings.Contains(errorString, string(MoreThanOneRemoteUserFound)) {
		return true
	}
	return false
}

/*
	This error must be used when there is no Secret
	corresponding to these requirements:
	- the secret referenced by the remoteuser must exists
	- the secret's type = kubernetes.io/basic-auth
*/

type CrendentialSearchErrorReason string

const (
	SecretNotFound         CrendentialSearchErrorReason = "no Secret found for the current user to log on the git repository with the RemoteUser: %s"                                                                   //nolint:lll
	MoreThanOneSecretFound CrendentialSearchErrorReason = "more than one Secret found for the current user with the RemoteUser: %s"                                                                                     //nolint:lll
	TokenNotFound          CrendentialSearchErrorReason = "no token found in the secret for the RemoteUser: %s; the token must be specified in the password field and the secret type must be kubernetes.io/basic-auth" //nolint:lll
)

type CrendentialSearchError struct {
	Reason     CrendentialSearchErrorReason
	RemoteUser syngit.RemoteUser
}

func (cse CrendentialSearchError) Error() string {
	return fmt.Sprintf(string(cse.Reason), cse.RemoteUser.Name)
}

func (cse CrendentialSearchError) ShouldContains(errorString string) bool {
	if strings.Contains(errorString, string(SecretNotFound)) {
		return true
	}
	if strings.Contains(errorString, string(MoreThanOneSecretFound)) {
		return true
	}
	if strings.Contains(errorString, string(TokenNotFound)) {
		return true
	}
	return false
}

/*
	This error must be used when the RemoteTarget
	does not match the RemoteSyncer's spec.
	Currently used on the defaultRemoteTarget.
*/

type RemoteTargetSearchError struct {
	UpstreamRepository string
	UpstreamBranch     string
	TargetRepository   string
	TargetBranch       string
}

func (rtse RemoteTargetSearchError) Error() string {
	return fmt.Sprintf(
		"the RemoteSyncer's repository or branch does not match the upstream repository or branch of the default RemoteTarget. RemoteSyncer repo: %s; RemoteSyncer branch: %s; RemoteTarget upstream repo: %s; RemoteTarget upstream branch: %s", //nolint:lll
		rtse.UpstreamRepository,
		rtse.UpstreamBranch,
		rtse.TargetRepository,
		rtse.TargetBranch,
	)
}

func (rtse RemoteTargetSearchError) ShouldContains(errorString string) bool {
	return strings.Contains(errorString, "the RemoteSyncer's repository or branch does not match the upstream repository or branch of the default RemoteTarget. RemoteSyncer repo: ") //nolint:lll
}

/*
	This error must be used when there is more than one
	RemoteTarget matching the upstream spec of the RemoteSyncer
	while the defined targetStrategy is "OneTarget".
*/

type MultipleTargetError struct {
	RemoteTargetsCount int
}

func (mte MultipleTargetError) Error() string {
	return "multiple RemoteTargets found for OneTarget set as the TargetStrategy in the RemoteSyncer"
}

func (mte MultipleTargetError) ShouldContains(errorString string) bool {
	return strings.Contains(errorString, mte.Error())
}

// ---

type LabelSeletorParsingErrorKind string

const (
	RemoteUserBindingSelectorError LabelSeletorParsingErrorKind = "error parsing the LabelSelector for the remoteUserBindingSelector: " //nolint:lll
	RemoteTargetSelectorError      LabelSeletorParsingErrorKind = "error parsing the LabelSelector for the remoteTargetSelector: "      //nolint:lll
)

type LabelSeletorParsingError struct {
	Kind       LabelSeletorParsingErrorKind
	LabelError error
}

func (lspe LabelSeletorParsingError) Error() string {
	return fmt.Sprintf("%s%s", lspe.Kind, lspe.LabelError)
}

func (lspe LabelSeletorParsingError) ShouldContains(errorString string) bool {
	if strings.Contains(errorString, string(RemoteUserBindingSelectorError)) {
		return true
	}
	if strings.Contains(errorString, string(RemoteTargetSelectorError)) {
		return true
	}
	return false
}

/*
	This error must be used when the dynamic interception
	webhook receives an empty request.
*/

type EmptyRequestError struct{}

func (ere EmptyRequestError) Error() string {
	return "the request is empty and it should not be"
}

func (ere EmptyRequestError) ShouldContains(errorString string) bool {
	return strings.Contains(errorString, ere.Error())
}

/*
	This error must be used when multiple
	RemoteUserBinding	with the same subject exist.
*/

type MultipleRemoteUserBindingError struct {
	RemoteUserBindingsCount int
}

func (mrube MultipleRemoteUserBindingError) Error() string {
	return "multiple RemoteUserBinding found OR the name of the user is not unique; this version of the operator work with the name as unique identifier for users" //nolint:lll
}

func (mrube MultipleRemoteUserBindingError) ShouldContains(errorString string) bool {
	return strings.Contains(errorString, mrube.Error())
}

// ---

type GitUrlParseError struct {
	ParseError error
}

func (gupe GitUrlParseError) Error() string {
	return fmt.Sprintf("error parsing git repository URL: %s", gupe.ParseError)
}

func (gupe GitUrlParseError) ShouldContains(errorString string) bool {
	return strings.Contains(errorString, "error parsing git repository URL")
}

// ---

type NonUniqueUserError struct {
	UserCount int
}

func (nuue NonUniqueUserError) Error() string {
	return "the name of the user is not unique; this version of the operator work with the name as unique identifier for users" //nolint:lll
}

func (nuue NonUniqueUserError) ShouldContains(errorString string) bool {
	return strings.Contains(errorString, nuue.Error())
}

// ---

type WrongYamlFormatError struct {
	Yaml string
}

func (wyfe WrongYamlFormatError) Error() string {
	return "failed to convert the excludedFields from the ConfigMap (wrong yaml format)"
}

func (wyfe WrongYamlFormatError) ShouldContains(errorString string) bool {
	return strings.Contains(errorString, wyfe.Error())
}
