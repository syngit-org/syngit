package controller

const (
	WebhookServiceName = "syngit-webhook-service"
	certificateName    = "syngit-webhook-cert"
	certPath           = "/tmp/k8s-webhook-server/serving-certs/tls.crt"
)

const (
	partiallyBoundMessage = "Some of the remote users are not bound"
	boundMessage          = "Every remote users are bound"
)
