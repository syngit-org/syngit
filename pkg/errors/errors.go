package errors

import (
	"fmt"
	"strings"

	syngit "github.com/syngit-org/syngit/pkg/api/v1beta4"
	authv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
)

// This error should be used when the user is not allowed to
// select a set of resources for interception.
func NewResourceScopeForbidden(user authv1.UserInfo, forbiddenResources []string) *resourceScopeForbidden {
	return &resourceScopeForbidden{User: user, ForbiddenResources: forbiddenResources}
}

type resourceScopeForbidden struct {
	User               authv1.UserInfo
	ForbiddenResources []string
}

func (e *resourceScopeForbidden) Error() string {
	return fmt.Sprintf("resource scope forbidden: the user %s is not allowed to scope: \n- %s",
		e.User,
		strings.Join(e.ForbiddenResources, "\n- "),
	)
}

func (e *resourceScopeForbidden) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "resource scope forbidden")
}

func (e *resourceScopeForbidden) Unwrap() error {
	return ErrResourceScopeForbidden
}

// The user is not allowed to select a set of resources for interception.
var ErrResourceScopeForbidden = &resourceScopeForbidden{}

// This error should be used when the user is not allowed to get its own RemoteUser.
func NewRemoteUserDenied(user authv1.UserInfo, remoteUserRef v1.ObjectReference) *denyGetRemoteUser {
	return &denyGetRemoteUser{User: user, RemoteUserRef: remoteUserRef}
}

type denyGetRemoteUser struct {
	User          authv1.UserInfo
	RemoteUserRef v1.ObjectReference
}

func (e *denyGetRemoteUser) Error() string {
	return fmt.Sprintf("get remote user denied: the user %s is not allowed to get the referenced remoteuser: %s",
		e.User,
		e.RemoteUserRef.Name,
	)
}

func (e *denyGetRemoteUser) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "get remote user denied")
}

func (e *denyGetRemoteUser) Unwrap() error {
	return ErrRemoteUserDenied
}

// The user is not allowed to get its own RemoteUser.
var ErrRemoteUserDenied = &denyGetRemoteUser{}

// This error should be used when there is no RemoteUserBinding
// corresponding to the specs (no reference to the user as the
// subject, no default found, ...).
func NewRemoteUserBindingNotFound(username string) *remoteUserBindingNotFound {
	return &remoteUserBindingNotFound{Username: username}
}

type remoteUserBindingNotFound struct {
	Username string
}

func (e *remoteUserBindingNotFound) Error() string {
	return fmt.Sprintf("remote user binding not found: no RemoteUserBinding found for the user %s",
		e.Username,
	)
}

func (e *remoteUserBindingNotFound) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "remote user binding not found")
}

func (e *remoteUserBindingNotFound) Unwrap() error {
	return ErrRemoteUserBindingNotFound
}

// There is no RemoteUserBinding corresponding to the specs
// (no reference to the user as the subject, no default found, ...).
var ErrRemoteUserBindingNotFound = &remoteUserBindingNotFound{}

// This error should be used when the RemoteTarget config is wrong
// or does not match the criteria.
func NewWrongRemoteTargetConfig(remoteSyncer syngit.RemoteSyncer, remoteUser syngit.RemoteUser) *wrongRemoteTargetConfig { // nolint:lll
	return &wrongRemoteTargetConfig{RemoteSyncer: remoteSyncer, RemoteUser: remoteUser}
}

type wrongRemoteTargetConfig struct {
	RemoteSyncer syngit.RemoteSyncer
	RemoteUser   syngit.RemoteUser
}

func (e *wrongRemoteTargetConfig) Error() string {
	return fmt.Sprintf("wrong remote target config: the fqdn of the default RemoteUser does not match the associated RemoteSyncer (%s) fqdn (%s)", // nolint:lll
		e.RemoteSyncer.Name,
		e.RemoteUser.Name,
	)
}

func (e *wrongRemoteTargetConfig) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "wrong remote target config")
}

func (e *wrongRemoteTargetConfig) Unwrap() error {
	return ErrWrongRemoteTargetConfig
}

// The RemoteTarget config is wrong or does not match the criteria.
var ErrWrongRemoteTargetConfig = &wrongRemoteTargetConfig{}

// This error should be used when the RemoteSyncer config is wrong
// or does not match the criteria.
func NewWrongRemoteSyncerConfig(message string) *wrongRemoteSyncerConfig {
	return &wrongRemoteSyncerConfig{Message: message}
}

type wrongRemoteSyncerConfig struct {
	Message string
}

func (e *wrongRemoteSyncerConfig) Error() string {
	return "wrong remote syncer config: " + e.Message
}

func (e *wrongRemoteSyncerConfig) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "wrong remote syncer config")
}

func (e *wrongRemoteSyncerConfig) Unwrap() error {
	return ErrWrongRemoteSyncerConfig
}

// The RemoteSyncer config is wrong or does not match the criteria.
var ErrWrongRemoteSyncerConfig = &wrongRemoteSyncerConfig{}

// This error should be used when the RemoteTarget
// is not found based on the criteria.
func NewRemoteTargetNotFound(details string) *remoteTargetNotFound {
	return &remoteTargetNotFound{Details: details}
}

type remoteTargetNotFound struct {
	Details string
}

func (e *remoteTargetNotFound) Error() string {
	return "no remote target found: " + e.Details
}

func (e *remoteTargetNotFound) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "no remote target found")
}

func (e *remoteTargetNotFound) Unwrap() error {
	return ErrRemoteTargetNotFound
}

// The RemoteTarget is not found based on the criteria.
var ErrRemoteTargetNotFound = &remoteTargetNotFound{}

// This error should be used when the RemoteUser
// is not found based on the criteria.
func NewRemoteUserNotFound(details string) *remoteUserNotFound {
	return &remoteUserNotFound{
		Details: details,
	}
}

type remoteUserNotFound struct {
	Details string
}

func (e *remoteUserNotFound) Error() string {
	return fmt.Sprintf("remote user not found: %s", e.Details)
}

func (e *remoteUserNotFound) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "remote user not found")
}

func (e *remoteUserNotFound) Unwrap() error {
	return ErrRemoteUserNotFound
}

// The RemoteUser is not found based on the criteria.
var ErrRemoteUserNotFound = &remoteUserNotFound{}

// This error should be used when the secret or the
// credential is not found.
func NewCredentialsNotFound(details, secretName string) *credentialsNotFound {
	return &credentialsNotFound{
		Details:    details,
		SecretName: secretName,
	}
}

type credentialsNotFound struct {
	Details    string
	SecretName string
}

func (e *credentialsNotFound) Error() string {
	return fmt.Sprintf("credential not found (secret name: \"%s\"): %s", e.SecretName, e.Details)
}

func (e *credentialsNotFound) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "credential not found")
}

func (e *credentialsNotFound) Unwrap() error {
	return ErrCredentialsNotFound
}

// Credentials not found.
var ErrCredentialsNotFound = &credentialsNotFound{}

// This error should be used when there is too much
// RemoteTarget matching the criteria.
func NewTooMuchRemoteTarget(details string, remoteTargetCount int) *tooMuchRemoteTarget {
	return &tooMuchRemoteTarget{Details: details, RemoteTargetsCount: remoteTargetCount}
}

type tooMuchRemoteTarget struct {
	Details            string
	RemoteTargetsCount int
}

func (e *tooMuchRemoteTarget) Error() string {
	return fmt.Sprintf("too much remote target found (%d): %s", e.RemoteTargetsCount, e.Details)
}

func (e *tooMuchRemoteTarget) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "too much remote target found")
}

func (e *tooMuchRemoteTarget) Unwrap() error {
	return ErrTooMuchRemoteTarget
}

// There is too much RemoteTarget matching the criteria.
var ErrTooMuchRemoteTarget = &tooMuchRemoteTarget{}

// This error should be used on label parsing error.
func NewWrongLabelParsing(details string) *wrongLabelParsing {
	return &wrongLabelParsing{
		Details: details,
	}
}

type wrongLabelParsing struct {
	Details string
}

func (e *wrongLabelParsing) Error() string {
	return fmt.Sprintf("wrong label parsing: %s", e.Details)
}

func (e *wrongLabelParsing) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "wrong label parsing")
}

func (e *wrongLabelParsing) Unwrap() error {
	return ErrWrongLabelParsing
}

// Label parsing error.
var ErrWrongLabelParsing = &wrongLabelParsing{}

// This error should be used when there is too much
// RemoteUserBinding matching the criteria.
func NewTooMuchRemoteUserBinding(details string, remoteUserBindingCount int) *tooMuchRemoteUserBinding {
	return &tooMuchRemoteUserBinding{
		Details:                 details,
		RemoteUserBindingsCount: remoteUserBindingCount,
	}
}

type tooMuchRemoteUserBinding struct {
	Details                 string
	RemoteUserBindingsCount int
}

func (e *tooMuchRemoteUserBinding) Error() string {
	return fmt.Sprintf("too much remote user binding found (%d): %s", e.RemoteUserBindingsCount, e.Details)
}

func (e *tooMuchRemoteUserBinding) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "too much remote user binding found")
}

func (e *tooMuchRemoteUserBinding) Unwrap() error {
	return ErrTooMuchRemoteUserBinding
}

// There is too much RemoteUserBinding matching the criteria.
var ErrTooMuchRemoteUserBinding = &tooMuchRemoteUserBinding{}

// This error should be used when there is too much
// RemoteUser matching the criteria.
func NewTooMuchRemoteUser(details string, remoteUserCount int) *tooMuchRemoteUser {
	return &tooMuchRemoteUser{
		RemoteUsersCount: remoteUserCount,
		Details:          details,
	}
}

type tooMuchRemoteUser struct {
	RemoteUsersCount int
	Details          string
}

func (e *tooMuchRemoteUser) Error() string {
	return fmt.Sprintf("too much remote user found (%d): %s", e.RemoteUsersCount, e.Details)
}

func (e *tooMuchRemoteUser) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "too much remote user found")
}

func (e *tooMuchRemoteUser) Unwrap() error {
	return ErrTooMuchRemoteUser
}

// There is too much RemoteUser matching the criteria.
var ErrTooMuchRemoteUser = &tooMuchRemoteUser{}

// This error should be used when there is too much
// subject matching the criteria.
func NewTooMuchSubject(details string) *tooMuchSubject {
	return &tooMuchSubject{
		Details: details,
	}
}

type tooMuchSubject struct {
	Details string
}

func (e *tooMuchSubject) Error() string {
	return fmt.Sprintf("too much subjects: %s", e.Details)
}

func (e *tooMuchSubject) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "too much subjects")
}

func (e *tooMuchSubject) Unwrap() error {
	return ErrTooMuchSubject
}

// There is too much subject matching the criteria.
var ErrTooMuchSubject = &tooMuchSubject{}

// This error should be used when the YAML cannot be parsed.
func NewWrongYAMLFormat(details string) *wrongYamlFormat {
	return &wrongYamlFormat{
		Details: details,
	}
}

type wrongYamlFormat struct {
	Details string
}

func (e *wrongYamlFormat) Error() string {
	return "wrong yaml format"
}

func (e *wrongYamlFormat) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "wrong yaml format")
}

func (e *wrongYamlFormat) Unwrap() error {
	return ErrWrongYAMLFormat
}

// The YAML cannot be parsed.
var ErrWrongYAMLFormat = &wrongYamlFormat{}

// This error should be used in the git pipeline.
func NewGitPipeline(details string) *gitPipeline {
	return &gitPipeline{Details: details}
}

type gitPipeline struct {
	Details string
}

func (e *gitPipeline) Error() string {
	return fmt.Sprintf("git pipeline processing error: %s", e.Details)
}

func (e *gitPipeline) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "git pipeline processing error")
}

func (e *gitPipeline) Unwrap() error {
	return ErrGitPipeline
}

// The git pipeline process has errored.
var ErrGitPipeline = &gitPipeline{}

// This error should be used in the interceptor pipeline.
func NewInterceptorPipeline(details string) *interceptorPipeline {
	return &interceptorPipeline{Details: details}
}

func BuildInterceptorPipelineErr(details string) string {
	return NewInterceptorPipeline(details).Error()
}

type interceptorPipeline struct {
	Details string
}

func (e *interceptorPipeline) Error() string {
	return fmt.Sprintf("interceptor pipeline processing error: %s", e.Details)
}

func (e *interceptorPipeline) ShouldContains(err error) bool {
	return strings.Contains(err.Error(), "interceptor pipeline processing error")
}

func (e *interceptorPipeline) Unwrap() error {
	return ErrInterceptorPipeline
}

// The interceptor pipeline process has errored.
var ErrInterceptorPipeline = &interceptorPipeline{}
