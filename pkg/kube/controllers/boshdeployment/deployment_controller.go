package boshdeployment

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	bdm "code.cloudfoundry.org/cf-operator/pkg/bosh/manifest"
	"code.cloudfoundry.org/cf-operator/pkg/kube/apis"
	bdv1 "code.cloudfoundry.org/cf-operator/pkg/kube/apis/boshdeployment/v1alpha1"
	"code.cloudfoundry.org/cf-operator/pkg/kube/util/config"
	"code.cloudfoundry.org/cf-operator/pkg/kube/util/ctxlog"
	"code.cloudfoundry.org/cf-operator/pkg/kube/util/reference"
)

// AddDeployment creates a new BOSHDeployment Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func AddDeployment(ctx context.Context, config *config.Config, mgr manager.Manager) error {
	ctx = ctxlog.NewContextWithRecorder(ctx, "boshdeployment-reconciler", mgr.GetRecorder("boshdeployment-recorder"))
	r := NewDeploymentReconciler(
		ctx, config, mgr,
		bdm.NewResolver(mgr.GetClient(), func() bdm.Interpolator { return bdm.NewInterpolator() }),
		controllerutil.SetControllerReference,
	)

	// Create a new controller
	c, err := controller.New("boshdeployment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource BOSHDeployment
	err = c.Watch(&source.Kind{Type: &bdv1.BOSHDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			return enqueueReconcilesForBoshDeployment(ctx, mgr, a)
		}),
	}, GetPredicates(ctx, mgr, &corev1.ConfigMap{}))
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			return enqueueReconcilesForBoshDeployment(ctx, mgr, a)

		}),
	}, GetPredicates(ctx, mgr, &corev1.Secret{}))
	if err != nil {
		return err
	}

	return nil
}

func enqueueReconcilesForBoshDeployment(ctx context.Context, mgr manager.Manager, a handler.MapObject) []reconcile.Request {
	var (
		listReconciles []reconcile.Request
		err            error
	)

	logReconcilePredicates := func(reconciles []reconcile.Request) {
		for _, reconciliation := range reconciles {
			ctxlog.WithEvent(a.Object, "Mapping").DebugJSON(ctx,
				"Enqueuing reconcile requests in response to events",
				ctxlog.ReconcileEventsFromSource{
					ReconciliationObjectName: reconciliation.Name,
					ReconciliationObjectKind: string(reference.ReconcileForBOSHDeployment),
					PredicateObjectName:      a.Meta.GetName(),
					PredicateObjectKind:      a.Object.GetObjectKind().GroupVersionKind().GroupKind().Kind,
					Namespace:                reconciliation.Namespace,
					Type:                     "mapping",
					Message:                  fmt.Sprintf("fan-out updates from %s into %s", a.Meta.GetName(), reconciliation.Name),
				})
		}
	}

	switch a.Object.(type) {
	case *corev1.ConfigMap:
		listReconciles, err = reference.GetReconciles(ctx, mgr.GetClient(), reference.ReconcileForBOSHDeployment, a.Object.(*corev1.ConfigMap))
		if err != nil {
			ctxlog.Errorf(ctx, "Failed to calculate reconciles for configmap '%s': %v", a.Object.(*corev1.ConfigMap).Name, err)
		}
		logReconcilePredicates(listReconciles)
	case *corev1.Secret:
		listReconciles, err = reference.GetReconciles(ctx, mgr.GetClient(), reference.ReconcileForBOSHDeployment, a.Object.(*corev1.Secret))
		if err != nil {
			ctxlog.Errorf(ctx, "Failed to calculate reconciles for secret '%s': %v", a.Object.(*corev1.Secret).Name, err)
		}
		logReconcilePredicates(listReconciles)
	}
	return listReconciles
}

// GetPredicates ...
func GetPredicates(ctx context.Context, mgr manager.Manager, customObject apis.Object) predicate.Funcs {

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			switch customObject.(type) {
			case *corev1.ConfigMap:
				c := e.Object.(*corev1.ConfigMap)
				reconciles, err := reference.GetReconciles(ctx, mgr.GetClient(), reference.ReconcileForBOSHDeployment, c)
				if err != nil {
					ctxlog.Errorf(ctx, "Failed to calculate reconciles for configMap '%s': %v", c.Name, err)
				}
				return len(reconciles) > 0
			case *corev1.Secret:
				s := e.Object.(*corev1.Secret)
				reconciles, err := reference.GetReconciles(ctx, mgr.GetClient(), reference.ReconcileForBOSHDeployment, s)
				if err != nil {
					ctxlog.Errorf(ctx, "Failed to calculate reconciles for secret '%s': %v", s.Name, err)
				}
				return len(reconciles) > 1
			default:
				return false
			}
		},
		DeleteFunc:  func(e event.DeleteEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			switch customObject.(type) {
			case *corev1.ConfigMap:
				cO := e.ObjectOld.(*corev1.ConfigMap)
				cN := e.ObjectNew.(*corev1.ConfigMap)
				reconciles, err := reference.GetReconciles(ctx, mgr.GetClient(), reference.ReconcileForBOSHDeployment, cN)
				if err != nil {
					ctxlog.Errorf(ctx, "Failed to calculate reconciles for configMap '%s': %v", cN.Name, err)
				}
				return len(reconciles) > 0 && !reflect.DeepEqual(cO.Data, cN.Data)
			case *corev1.Secret:
				sO := e.ObjectOld.(*corev1.Secret)
				sN := e.ObjectNew.(*corev1.Secret)
				reconciles, err := reference.GetReconciles(ctx, mgr.GetClient(), reference.ReconcileForBOSHDeployment, sN)
				if err != nil {
					ctxlog.Errorf(ctx, "Failed to calculate reconciles for secret '%s': %v", sN.Name, err)
				}
				return len(reconciles) > 1 && !reflect.DeepEqual(sO.Data, sN.Data)
			default:
				return false
			}
		},
	}
}
