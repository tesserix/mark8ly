// Package k8sprov provisions Kubernetes resources (Certificate, Gateway,
// VirtualService) for verified custom domains. Uses the dynamic client so
// we don't take a dependency on cert-manager or Istio type packages.
//
// This package is best-effort: errors are returned but the caller (domain
// service) should log + continue rather than fail the user-visible verify
// flow on transient cluster issues.
package k8sprov

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	istioIngressNS         = "istio-ingress"
	storefrontNS           = "mark8ly"
	storefrontService      = "mark8ly-storefront.mark8ly.svc.cluster.local"
	storefrontPort         = 4203
	clusterIssuer          = "letsencrypt-custom-domain"
	istioGatewaySelector   = "custom-ingressgateway"
	managedByLabel         = "mark8ly-marketplace-api"
	customDomainLabel      = "custom-domain"
)

var (
	gvrCertificate = schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificates",
	}
	gvrGateway = schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1",
		Resource: "gateways",
	}
	gvrVirtualService = schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1",
		Resource: "virtualservices",
	}
	gvrAuthorizationPolicy = schema.GroupVersionResource{
		Group:    "security.istio.io",
		Version:  "v1",
		Resource: "authorizationpolicies",
	}
)

// Provisioner creates and deletes the per-domain k8s resources.
type Provisioner struct {
	dyn    dynamic.Interface
	logger *slog.Logger
}

// New constructs a Provisioner. Returns nil, nil when k8s config is
// unavailable (e.g. running outside a cluster during local dev) so the
// caller can skip provisioning gracefully.
func New(logger *slog.Logger) (*Provisioner, error) {
	cfg, err := loadConfig()
	if err != nil {
		if logger != nil {
			logger.Warn("k8sprov: kube config unavailable, provisioning disabled", "err", err)
		}
		return nil, nil
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8sprov: dynamic client: %w", err)
	}
	return &Provisioner{dyn: dyn, logger: logger}, nil
}

func loadConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	// Fall back to local kubeconfig for dev.
	loader := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loader,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
}

// ProvisionResult describes what was created.
type ProvisionResult struct {
	CertSecretName string
}

// Provision creates Certificate + Gateway + VirtualService for a
// verified custom domain. Idempotent: existing resources are updated
// rather than recreated.
func (p *Provisioner) Provision(ctx context.Context, domain string) (*ProvisionResult, error) {
	slug := domainSlug(domain)
	certName := slug + "-tls"
	gatewayName := slug + "-gateway"
	routeName := slug + "-route"
	authzName := "allow-custom-domain-" + slug

	// Covers apex + wildcard so merchants can use subdomains without
	// adding more records later.
	hosts := []string{domain, "*." + domain}

	if err := p.applyCertificate(ctx, certName, hosts); err != nil {
		return nil, fmt.Errorf("certificate: %w", err)
	}
	if err := p.applyGateway(ctx, gatewayName, slug, certName, hosts); err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	if err := p.applyVirtualService(ctx, routeName, slug, gatewayName, hosts); err != nil {
		return nil, fmt.Errorf("virtualservice: %w", err)
	}
	if err := p.applyAuthorizationPolicy(ctx, authzName, slug, hosts); err != nil {
		return nil, fmt.Errorf("authorizationpolicy: %w", err)
	}

	return &ProvisionResult{CertSecretName: certName}, nil
}

// Deprovision deletes the per-domain k8s resources. Best-effort: missing
// resources are ignored.
func (p *Provisioner) Deprovision(ctx context.Context, domain string) error {
	slug := domainSlug(domain)

	type target struct {
		gvr       schema.GroupVersionResource
		namespace string
		name      string
	}
	targets := []target{
		{gvrAuthorizationPolicy, istioIngressNS, "allow-custom-domain-" + slug},
		{gvrVirtualService, storefrontNS, slug + "-route"},
		{gvrGateway, istioIngressNS, slug + "-gateway"},
		{gvrCertificate, istioIngressNS, slug + "-tls"},
	}

	var firstErr error
	for _, t := range targets {
		err := p.dyn.Resource(t.gvr).Namespace(t.namespace).
			Delete(ctx, t.name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			if p.logger != nil {
				p.logger.Error("k8sprov: delete failed", "kind", t.gvr.Resource, "name", t.name, "err", err)
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// CertStatus returns the current cert-manager Certificate status.
// Returns ready=true when the certificate has been issued and the
// secret is available for the gateway to serve.
func (p *Provisioner) CertStatus(ctx context.Context, domain string) (ready bool, message string, err error) {
	slug := domainSlug(domain)
	certName := slug + "-tls"
	obj, err := p.dyn.Resource(gvrCertificate).Namespace(istioIngressNS).
		Get(ctx, certName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, "certificate not found", nil
		}
		return false, "", err
	}

	conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, c := range conditions {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cm["type"] == "Ready" {
			status, _ := cm["status"].(string)
			msg, _ := cm["message"].(string)
			return status == "True", msg, nil
		}
	}
	return false, "pending", nil
}

func (p *Provisioner) applyCertificate(ctx context.Context, name string, hosts []string) error {
	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata":   commonMeta(name, istioIngressNS, labelsFor(name)),
			"spec": map[string]interface{}{
				"secretName": name,
				"dnsNames":   toIfaceSlice(hosts),
				"issuerRef": map[string]interface{}{
					"kind": "ClusterIssuer",
					"name": clusterIssuer,
				},
			},
		},
	}
	return p.upsert(ctx, gvrCertificate, istioIngressNS, u)
}

func (p *Provisioner) applyGateway(ctx context.Context, name, slug, certName string, hosts []string) error {
	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1",
			"kind":       "Gateway",
			"metadata":   commonMeta(name, istioIngressNS, labelsFor(slug)),
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{"istio": istioGatewaySelector},
				"servers": []interface{}{
					map[string]interface{}{
						"hosts": toIfaceSlice(hosts),
						"port": map[string]interface{}{
							"name":     "https-" + slug,
							"number":   int64(443),
							"protocol": "HTTPS",
						},
						"tls": map[string]interface{}{
							"mode":               "SIMPLE",
							"credentialName":     certName,
							"minProtocolVersion": "TLSV1_2",
						},
					},
					map[string]interface{}{
						"hosts": toIfaceSlice(hosts),
						"port": map[string]interface{}{
							"name":     "http-" + slug,
							"number":   int64(80),
							"protocol": "HTTP",
						},
						"tls": map[string]interface{}{"httpsRedirect": true},
					},
				},
			},
		},
	}
	return p.upsert(ctx, gvrGateway, istioIngressNS, u)
}

func (p *Provisioner) applyVirtualService(ctx context.Context, name, slug, gatewayName string, hosts []string) error {
	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1",
			"kind":       "VirtualService",
			"metadata":   commonMeta(name, storefrontNS, labelsFor(slug)),
			"spec": map[string]interface{}{
				"gateways": []interface{}{istioIngressNS + "/" + gatewayName},
				"hosts":    toIfaceSlice(hosts),
				"http": []interface{}{
					map[string]interface{}{
						"route": []interface{}{
							map[string]interface{}{
								"destination": map[string]interface{}{
									"host": storefrontService,
									"port": map[string]interface{}{
										"number": int64(storefrontPort),
									},
								},
							},
						},
						"timeout": "30s",
					},
				},
			},
		},
	}
	return p.upsert(ctx, gvrVirtualService, storefrontNS, u)
}

// applyAuthorizationPolicy creates the Istio AuthorizationPolicy that
// allowlists traffic for the custom domain on the custom-ingressgateway.
// Without this, the gateway rejects all requests with 403.
func (p *Provisioner) applyAuthorizationPolicy(ctx context.Context, name, slug string, hosts []string) error {
	hostRules := make([]interface{}, 0, len(hosts))
	for _, h := range hosts {
		hostRules = append(hostRules, map[string]interface{}{
			"to": []interface{}{
				map[string]interface{}{
					"operation": map[string]interface{}{
						"hosts": []interface{}{h},
					},
				},
			},
		})
	}
	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "security.istio.io/v1",
			"kind":       "AuthorizationPolicy",
			"metadata":   commonMeta(name, istioIngressNS, labelsFor(slug)),
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"istio": istioGatewaySelector,
					},
				},
				"action": "ALLOW",
				"rules":  hostRules,
			},
		},
	}
	return p.upsert(ctx, gvrAuthorizationPolicy, istioIngressNS, u)
}

func (p *Provisioner) upsert(ctx context.Context, gvr schema.GroupVersionResource, namespace string, obj *unstructured.Unstructured) error {
	name := obj.GetName()
	existing, err := p.dyn.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = p.dyn.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	_, err = p.dyn.Resource(gvr).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func commonMeta(name, namespace string, labels map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"name":      name,
		"namespace": namespace,
		"labels":    labels,
	}
}

func labelsFor(slug string) map[string]interface{} {
	return map[string]interface{}{
		"app.kubernetes.io/managed-by": managedByLabel,
		customDomainLabel:              slug,
	}
}

// domainSlug converts "shop.primasyss.com" → "shop-primasyss-com".
// All k8s resource names for this domain derive from the slug.
func domainSlug(domain string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(domain)), ".", "-")
}

func toIfaceSlice(s []string) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
