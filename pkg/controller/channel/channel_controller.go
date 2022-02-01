// Copyright 2021 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package channel

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	spokeClusterV1 "open-cluster-management.io/api/cluster/v1"
	chv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	"open-cluster-management.io/multicloud-operators-channel/pkg/utils"

	"github.com/go-logr/logr"
	gerr "github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metaerr "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	clusterRules = []rbac.PolicyRule{
		{
			Verbs:     []string{"get", "list", "watch"},
			APIGroups: []string{chv1.SchemeGroupVersion.Group},
			Resources: []string{"channels", "channels/status"},
		},
		{
			Verbs:     []string{"get", "list", "watch"},
			APIGroups: []string{""},
			Resources: []string{"secrets", "configmaps"},
		},
	}

	srtGvk = schema.GroupVersionKind{Group: "", Kind: "Secret", Version: "v1"}
	cmGvk  = schema.GroupVersionKind{Group: "", Kind: "ConfigMap", Version: "v1"}
)

const (
	clusterCRDName  = "clusters.clusterregistry.k8s.io"
	controllerName  = "channel"
	controllerSetup = "channel-setup"
	backupLabel     = "cluster.open-cluster-management.io/backup"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Channel Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, dynamicClient dynamic.Interface, recorder record.EventRecorder, logger logr.Logger,
	channelDescriptor *utils.ChannelDescriptor) error {
	return add(mgr, newReconciler(mgr, dynamicClient, recorder, logger.WithName(controllerName)), logger.WithName(controllerSetup))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, dynamicClient dynamic.Interface, recorder record.EventRecorder,
	logger logr.Logger) reconcile.Reconciler {
	return &ReconcileChannel{
		Client:        mgr.GetClient(),
		DynamicClient: dynamicClient,
		scheme:        mgr.GetScheme(),
		Recorder:      recorder,
		Log:           logger,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler, logger logr.Logger) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	logger.Info("failed to add CRD scheme to manager")

	if err := apiextensionsv1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		logger.Error(err, "failed to add CRD scheme to manager")
	}
	// Watch for changes to Channel
	err = c.Watch(&source.Kind{Type: &chv1.Channel{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	if utils.IsReadyClusterRegistry(mgr.GetAPIReader()) {
		cmapper := &clusterMapper{Client: mgr.GetClient(), logger: logger}
		err = c.Watch(
			&source.Kind{Type: &spokeClusterV1.ManagedCluster{}},
			handler.EnqueueRequestsFromMapFunc(cmapper.Map),
			utils.ClusterPredicateFunc,
		)
	}

	return err
}

type clusterMapper struct {
	client.Client
	logger logr.Logger
}

// Map triggers all placements
func (mapper *clusterMapper) Map(obj client.Object) []reconcile.Request {
	cname := obj.GetName()

	mapper.logger.Info(fmt.Sprintf("In cluster Mapper for %v", cname))

	plList := &chv1.ChannelList{}

	listopts := &client.ListOptions{}
	err := mapper.List(context.TODO(), plList, listopts)

	if err != nil {
		mapper.logger.Error(err, "failed to list channels")
	}

	var requests []reconcile.Request

	for _, pl := range plList.Items {
		objkey := types.NamespacedName{
			Name:      pl.GetName(),
			Namespace: pl.GetNamespace(),
		}

		requests = append(requests, reconcile.Request{NamespacedName: objkey})
	}

	return requests
}

var _ reconcile.Reconciler = &ReconcileChannel{}

// ReconcileChannel reconciles a Channel object
type ReconcileChannel struct {
	client.Client
	DynamicClient dynamic.Interface
	scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	Log           logr.Logger
}

// Reconcile reads that state of the cluster for a Channel object and makes changes based on the state read
// and what is in the Channel.Spec
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.open-cluster-management.io,resources=channels/status,verbs=get;update;patch
func (r *ReconcileChannel) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := r.Log.WithValues("channel-reconcile", request.NamespacedName)

	log.Info(fmt.Sprintf("Starting %v reconcile loop for %v", controllerName, request.NamespacedName))
	defer log.Info(fmt.Sprintf("Finish %v reconcile loop for %v", controllerName, request.NamespacedName))

	instance := &chv1.Channel{}

	err := r.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if kerr.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			//sync the channel to the serving-channel annotation in all involved secrets - remove channel
			if err := r.syncReferredObjAnnotationLabel(request, nil, srtGvk, log); err != nil {
				return reconcile.Result{}, err
			}

			//remove the channel from the serving-channel annotation in all involved ConfigMaps - remove channel
			if err := r.syncReferredObjAnnotationLabel(request, nil, cmGvk, log); err != nil {
				return reconcile.Result{}, err
			}

			return reconcile.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if (strings.EqualFold(string(instance.Spec.Type), chv1.ChannelTypeNamespace)) && (instance.Spec.Pathname != instance.GetNamespace()) {
		instance.Spec.Pathname = instance.GetNamespace()

		err := r.Update(context.TODO(), instance)
		if err != nil {
			log.Info(fmt.Sprintf("can't update the pathname field due to %v", err))
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	// find the channel controller pod namespace, it is running in the ACM namespece
	mchNamespace := r.FindMultiClusterHubNS(log)

	err = r.validateClusterRBAC(instance, log, mchNamespace)
	if err != nil {
		log.Error(err, fmt.Sprintf("failed to validate RBAC for clusters for channel %v", instance.Name))
		return reconcile.Result{}, err
	}

	r.handleReferencedObjects(instance, request, log)

	err = r.cleanRoleFromAcmNS(instance, log, mchNamespace)
	if err != nil {
		log.Error(err, "failed to clean up channel role/rolebinding in the ACM system NameSpace")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileChannel) handleReferencedObjects(instance *chv1.Channel, req reconcile.Request, log logr.Logger) {
	// If the channel has relative secret and configMap, annotate the channel info in the secret and configMap
	//sync the channel to the serving-channel annotation in all involved secrets.
	srtRef := instance.Spec.SecretRef

	if srtRef != nil {
		if srtRef.Namespace == "" {
			srtRef.Namespace = instance.GetNamespace()
		}

		if err := r.updatedReferencedObjectLabels(srtRef, srtGvk, log); err != nil {
			r.Log.Error(err, "failed to update referred secret label")
		}

		if err := r.syncReferredObjAnnotationLabel(req, srtRef, srtGvk, log); err != nil {
			r.Log.Error(err, "failed to annotate")
		}
	}

	//	//sync the channel to the serving-channel annotation in all involved ConfigMaps.
	cmRef := instance.Spec.ConfigMapRef
	if cmRef != nil {
		if cmRef.Namespace == "" {
			cmRef.Namespace = instance.GetNamespace()
		}

		if err := r.updatedReferencedObjectLabels(cmRef, cmGvk, log); err != nil {
			r.Log.Error(err, "failed to update referred configMap label")
		}

		if err := r.syncReferredObjAnnotationLabel(req, cmRef, cmGvk, log); err != nil {
			r.Log.Error(err, "failed to annotate")
		}
	}
}

func (r *ReconcileChannel) updatedReferencedObjectLabels(ref *corev1.ObjectReference, objGvk schema.GroupVersionKind, logger logr.Logger) error {
	if ref == nil {
		return gerr.New(fmt.Sprintf("empty referred object %v", objGvk.Kind))
	}

	objName := ref.Name
	objNs := ref.Namespace

	obj := &unstructured.Unstructured{}
	objKey := types.NamespacedName{Name: objName, Namespace: objNs}

	obj.SetGroupVersionKind(objGvk)

	if err := r.Get(context.TODO(), objKey, obj); err != nil {
		return gerr.Wrapf(err, "failed to get the reference object %v", objGvk.Kind)
	}

	localLabels := obj.GetLabels()
	if localLabels == nil {
		localLabels = make(map[string]string)
	}

	localLabels[chv1.ServingChannel] = "true"
	obj.SetLabels(localLabels)

	if err := r.Update(context.TODO(), obj); err != nil {
		return gerr.Wrapf(err, "failed to update the referred object %v", objGvk.Kind)
	}

	logger.Info(fmt.Sprintf("Set label serving-channel to object: %v", objKey.String()))

	return nil
}

func (r *ReconcileChannel) syncReferredObjAnnotationLabel(
	rq reconcile.Request,
	ref *corev1.ObjectReference, objGvk schema.GroupVersionKind, logger logr.Logger) error {
	chnKey := types.NamespacedName{Name: rq.Name, Namespace: rq.Namespace}

	uObjList := &unstructured.UnstructuredList{}

	uObjList.SetGroupVersionKind(objGvk)

	opts := &client.ListOptions{}

	objLabel := make(map[string]string)
	objLabel[chv1.ServingChannel] = "true"
	labelSelector := &metav1.LabelSelector{
		MatchLabels: objLabel,
	}

	clSelector, err := utils.ConvertLabels(labelSelector)
	if err != nil {
		return gerr.Wrap(err, "failed to set label selector for referred object")
	}

	opts.LabelSelector = clSelector

	if err := r.Client.List(context.TODO(), uObjList, opts); err != nil {
		return gerr.Wrapf(err, "failed to list objects %v. error: ", objGvk.String())
	}

	for _, obj := range uObjList.Items {
		obj := obj

		//set annotations
		annotations := obj.GetAnnotations()

		if annotations == nil {
			annotations = make(map[string]string)
		}

		newServingChannel := annotations[chv1.ServingChannel]

		if ref != nil && (ref.Name > "" && ref.Namespace > "") {
			if obj.GetName() == ref.Name && obj.GetNamespace() == ref.Namespace {
				newServingChannel = utils.UpdateServingChannel(annotations[chv1.ServingChannel], chnKey.String(), "add")
			}
		} else {
			newServingChannel = utils.UpdateServingChannel(annotations[chv1.ServingChannel], chnKey.String(), "remove")
		}

		if newServingChannel > "" {
			annotations[chv1.ServingChannel] = newServingChannel
		} else {
			delete(annotations, chv1.ServingChannel)
		}

		obj.SetAnnotations(annotations)

		//set labels
		labels := obj.GetLabels()

		if labels == nil {
			labels = make(map[string]string)
		}

		if _, ok := labels[backupLabel]; !ok {
			labels[backupLabel] = "app"
		}

		obj.SetLabels(labels)

		if err := r.Update(context.TODO(), &obj); err != nil {
			logger.Error(err, fmt.Sprintf("failed to annotate object: %v/%v", obj.GetNamespace(), obj.GetName()))
		}
	}

	return nil
}

func (r *ReconcileChannel) validateClusterRBAC(instance *chv1.Channel, logger logr.Logger, mchNamespace string) error {
	if instance.Namespace == mchNamespace {
		logger.Info(fmt.Sprintf("Don't create role and rolebinding as the channel %v/%v is in the ACM Namespace %v",
			instance.Namespace, instance.Name, mchNamespace))
		return nil
	}

	role := &rbac.Role{}

	if err := r.setupRole(instance, role); err != nil {
		return gerr.Wrap(err, "failed to create/update rolebinding")
	}

	rolebinding := &rbac.RoleBinding{}

	var subjects []rbac.Subject

	cllist := &spokeClusterV1.ManagedClusterList{}

	if err := r.List(context.TODO(), cllist, &client.ListOptions{}); err != nil {
		if metaerr.IsNoMatchError(err) {
			r.Log.Error(err, fmt.Sprintf("skipping the RBAC validation for %v/%v", instance.GetNamespace(), instance.GetName()))
			return nil
		}

		if kerr.IsNotFound(err) {
			return nil
		}

		return gerr.Wrap(err, "failed to list cluster resource while rolebinding")
	}

	for _, cl := range cllist.Items {
		subjects = append(subjects,
			rbac.Subject{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "User",
				Name:     "system:open-cluster-management:cluster:" + cl.Name + ":addon:application-manager:agent:appmgr",
			},
			rbac.Subject{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "system:open-cluster-management:cluster:" + cl.Name + ":addon:application-manager",
			})
	}

	roleref := rbac.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     instance.Name,
	}

	if err := r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, rolebinding); err != nil {
		if kerr.IsNotFound(err) {
			rolebinding.Name = instance.Name
			rolebinding.Namespace = instance.Namespace

			if err := controllerutil.SetControllerReference(instance, rolebinding, r.scheme); err != nil {
				return gerr.Wrap(err, "failed to set controller reference")
			}

			rolebinding.RoleRef = roleref
			rolebinding.Subjects = subjects

			if err := r.Create(context.TODO(), rolebinding); err != nil {
				return gerr.Wrap(err, "faild to create rolebinding")
			}

			return nil
		}

		return gerr.Wrap(err, "failed to get rolebinding state")
	}

	if !reflect.DeepEqual(subjects, rolebinding.Subjects) || !reflect.DeepEqual(rolebinding.RoleRef, roleref) {
		if err := controllerutil.SetControllerReference(instance, rolebinding, r.scheme); err != nil {
			return gerr.Wrap(err, "failed to set controller reference")
		}

		rolebinding.RoleRef = roleref
		rolebinding.Subjects = subjects

		if err := r.Update(context.TODO(), rolebinding); err != nil {
			return gerr.Wrap(err, "failed to update rolebinding")
		}
	}

	logger.Info(fmt.Sprintf("created role %v and rolebinding %v with subjects %v", role.Name, rolebinding.Name, rolebinding.Subjects))

	return nil
}

func (r *ReconcileChannel) setupRole(instance *chv1.Channel, role *rbac.Role) error {
	if err := r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, role); err != nil {
		if kerr.IsNotFound(err) {
			role.Name = instance.Name
			role.Namespace = instance.Namespace
			role.Rules = clusterRules

			if err := controllerutil.SetControllerReference(instance, role, r.scheme); err != nil {
				return gerr.Wrap(err, "failed to set controller reference for role set up")
			}

			if err := r.Create(context.TODO(), role); err != nil {
				return gerr.Wrapf(err, "failed to create role %v", role.Name)
			}

			return nil
		}

		return err
	}

	if !reflect.DeepEqual(role.Rules, clusterRules) {
		role.Rules = clusterRules

		if err := controllerutil.SetControllerReference(instance, role, r.scheme); err != nil {
			return gerr.Wrap(err, "failed to set controller reference for role set up")
		}

		if err := r.Update(context.TODO(), role); err != nil {
			return gerr.Wrapf(err, "failed to update role %v", role.Name)
		}
	}

	return nil
}

// Clean up channel role/rolebinding if the channel is located in the ACM system Namespace,
// so the ACM NameSpace Secrets won't be exposed to managed clusters.
// The channels created in the ACM system NS are only used by hub standalone subscriptions.
func (r *ReconcileChannel) cleanRoleFromAcmNS(instance *chv1.Channel, logger logr.Logger, mchNamespace string) error {
	if instance.Namespace != mchNamespace {
		logger.Info(fmt.Sprintf("The channel %v/%v is not in the ACM Namespace %v, skipping...",
			instance.Namespace, instance.Name, mchNamespace))
		return nil
	}

	role := &rbac.Role{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, role)

	if err == nil {
		if err = r.Delete(context.TODO(), role); err != nil {
			return gerr.Wrapf(err, "failed to delete role %v/%v", role.Namespace, role.Name)
		}
	}

	rolebinding := &rbac.RoleBinding{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, rolebinding)

	if err == nil {
		if err = r.Delete(context.TODO(), rolebinding); err != nil {
			return gerr.Wrapf(err, "failed to delete rolebinding %v/%v", rolebinding.Namespace, rolebinding.Name)
		}
	}

	return nil
}

func (r *ReconcileChannel) FindMultiClusterHubNS(logger logr.Logger) string {
	mchGVR := schema.GroupVersionResource{
		Group:    "operator.open-cluster-management.io",
		Version:  "v1",
		Resource: "multiclusterhubs",
	}

	objlist, _ := r.DynamicClient.Resource(mchGVR).List(context.TODO(), metav1.ListOptions{})

	if objlist == nil {
		logger.Info("No MultiClusterHub Resource found")
		return ""
	}

	if len(objlist.Items) == 1 {
		mchNS := objlist.Items[0].GetNamespace()
		logger.Info(fmt.Sprintf("ACM system Namespace found: %v", mchNS))

		return mchNS
	}

	logger.Info("There should be ONLY one MultiClusterHub object")

	return ""
}
