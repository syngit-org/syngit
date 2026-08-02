/*
Copyright 2026.

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

package controller

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// dynamicWebhookEntry is the single ValidatingWebhook entry that one syncer owns
// inside the shared dynamic ValidatingWebhookConfiguration.
type dynamicWebhookEntry struct {
	// name is unique within the configuration and identifies the owning syncer.
	name string
	// path the API server calls back on.
	path string

	rules             []admissionv1.RuleWithOperations
	namespaceSelector *v1.LabelSelector
	objectSelector    *v1.LabelSelector
}

// dynamicWebhookManager owns the shared ValidatingWebhookConfiguration that every
// syncer, namespaced or cluster-wide, appends its entry to. Both reconcilers
// embed it so that the certificate handling, the dev-mode URL and the
// read-modify-write of the shared object have exactly one implementation.
type dynamicWebhookManager struct {
	client.Client

	dynamicWebhookName string
	managerNamespace   string

	devMode        bool
	devWebhookHost string
	devWebhookCert string
	devWebhookPort string
}

func (d *dynamicWebhookManager) loadFromEnv(c client.Client) {
	d.Client = c
	d.devMode = os.Getenv("DEV_MODE") == "true" // nolint:goconst
	d.devWebhookHost = os.Getenv("DEV_WEBHOOK_HOST")
	d.devWebhookPort = os.Getenv("DEV_WEBHOOK_PORT")
	d.devWebhookCert = os.Getenv("DEV_WEBHOOK_CERT")
	d.managerNamespace = os.Getenv("MANAGER_NAMESPACE")
	d.dynamicWebhookName = os.Getenv("DYNAMIC_WEBHOOK_NAME")
}

// caCert reads the CA the API server must trust when calling back.
func (d *dynamicWebhookManager) caCert() ([]byte, error) {
	path := certPath
	if d.devMode {
		path = d.devWebhookCert
	}
	caCert, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read the cert file %s: %w", path, err)
	}
	return caCert, nil
}

// clientConfig points the API server at the webhook service in the manager
// namespace, or directly at the developer's host in dev mode.
func (d *dynamicWebhookManager) clientConfig(caCert []byte, path string) (admissionv1.WebhookClientConfig, map[string]string) {
	annotations := map[string]string{}

	if d.devMode {
		url := "https://" + d.devWebhookHost + ":" + d.devWebhookPort + path
		return admissionv1.WebhookClientConfig{URL: &url, CABundle: caCert}, annotations
	}

	annotations["cert-manager.io/inject-ca-from"] = fmt.Sprintf("%s:%s", d.managerNamespace, certificateName)

	return admissionv1.WebhookClientConfig{
		Service: &admissionv1.ServiceReference{
			Name:      WebhookServiceName,
			Namespace: d.managerNamespace,
			Path:      &path,
		},
		CABundle: caCert,
	}, annotations
}

// Brings the shared configuration in line with entry: it adds or updates
// the owning syncer's webhook, or removes it when isDeleted.
//
// The read-modify-write is retried on conflict because both the RemoteSyncer and
// the ClusterWideRemoteSyncer reconcilers write this one object; without the
// retry a concurrent reconcile would silently drop the other's entry.
func (d *dynamicWebhookManager) upsert(ctx context.Context, entry dynamicWebhookEntry, isDeleted bool) error {
	caCert, err := d.caCert()
	if err != nil {
		return err
	}
	clientConfig, annotations := d.clientConfig(caCert, entry.path)

	sideEffectsNone := admissionv1.SideEffectClassNone
	webhook := admissionv1.ValidatingWebhook{
		Name:                    entry.name,
		AdmissionReviewVersions: []string{"v1"},
		SideEffects:             &sideEffectsNone,
		Rules:                   entry.rules,
		ClientConfig:            clientConfig,
		NamespaceSelector:       entry.namespaceSelector,
		ObjectSelector:          entry.objectSelector,
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		found := &admissionv1.ValidatingWebhookConfiguration{}
		err := d.Get(ctx, types.NamespacedName{Name: d.dynamicWebhookName}, found)

		if apierrors.IsNotFound(err) {
			if isDeleted {
				return nil
			}
			// First syncer of the cluster: create the shared configuration.
			return d.Create(ctx, &admissionv1.ValidatingWebhookConfiguration{
				ObjectMeta: v1.ObjectMeta{
					Name:        d.dynamicWebhookName,
					Annotations: annotations,
				},
				Webhooks: []admissionv1.ValidatingWebhook{webhook},
			})
		}
		if err != nil {
			return err
		}

		// Rebuild the list without this syncer's entry, remembering whether the
		// entry we would write is already exactly what is there.
		var others []admissionv1.ValidatingWebhook
		upToDate := false
		for _, existing := range found.Webhooks {
			if existing.Name != entry.name {
				others = append(others, existing)
				continue
			}
			upToDate = webhookEntryEqual(existing, webhook)
		}

		if upToDate && !isDeleted {
			return nil
		}

		if !isDeleted {
			others = append(others, webhook)
		}
		found.Webhooks = others

		return d.Update(ctx, found)
	})
}

// Reports whether the entry already in the configuration matches the one we
// would write. Every field the syncer controls is compared: comparing only
// the rules would let a change to either selector go unapplied.
func webhookEntryEqual(a, b admissionv1.ValidatingWebhook) bool {
	return slices.EqualFunc(a.Rules, b.Rules, rulesAreEqual) &&
		reflect.DeepEqual(a.NamespaceSelector, b.NamespaceSelector) &&
		reflect.DeepEqual(a.ObjectSelector, b.ObjectSelector)
}
