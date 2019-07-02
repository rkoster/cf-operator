package extendedstatefulset

import (
	"go.uber.org/zap"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"code.cloudfoundry.org/cf-operator/pkg/kube/util/config"
)

// AddPod creates a new hook for working with Pods and adds it to the Manager
func AddPod(log *zap.SugaredLogger, config *config.Config, mgr manager.Manager, hookServer *webhook.Server) (*admission.Webhook, error) {
	log.Info("Creating the ExtendedStatefulSet Pod controller, setting up pod webhooks")

	podMutator := NewPodMutator(log, config, mgr, controllerutil.SetControllerReference)

	mutatingWebhook := &webhook.Admission{Handler: podMutator}
	hookServer.Register("/mutate-v1-pod", mutatingWebhook)

	// mutatingWebhook, err := builder.WebhookManagedBy(*mgr).
	//         Path("/mutate-pods").
	//         Mutating().
	//         NamespaceSelector(&metav1.LabelSelector{
	//                 MatchLabels: map[string]string{
	//                         "cf-operator-ns": config.Namespace,
	//                 },
	//         }).
	//         ForType(&corev1.Pod{}).
	//         Handlers(podMutator).
	//         WithManager(mgr).
	//         FailurePolicy(admissionregistrationv1beta1.Fail).
	//         Build()
	// if err != nil {
	//         return nil, errors.Wrap(err, "couldn't build a new webhook")
	// }

	return mutatingWebhook, nil
}
