/*


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

package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	kv1 "k8s.io/api/core/v1"
	kv2 "k8s.io/api/apps/v1"
	kv3 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	batchv1 "new-operator/api/v1"
)

// WebsiteReconciler reconciles a Website object
type WebsiteReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=batch.arbaaz.test.in,resources=websites,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch.arbaaz.test.in,resources=websites/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=api.apps.v1,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.core.v1,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.apps.v1,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.autoscaling.v2beta1,resources=hpa,verbs=get;list;watch;create;update;patch;delete

func (r *WebsiteReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("Website", req.NamespacedName)
	// If any changes occur then reconcile function will be called.
	// Get the website object on which reconcile is called
	var website batchv1.Website
	if err := r.Get(ctx, req.NamespacedName, &website); err != nil {
		//log.Error(err, "unable to fetch Website")
		log.Info("Unable to fetch Website", "Error", err)

		// Delete Deployment if it exists
		var websitesDeployment kv2.Deployment
		if err := r.Get(ctx, req.NamespacedName, &websitesDeployment); err == nil {
			return r.RemoveDeployment(ctx, &websitesDeployment, log)
		}

		// Delete service if it exists
		var websiteService kv1.Service
		if err := r.Get(ctx, req.NamespacedName, &websiteService); err == nil {
			return r.RemoveService(ctx, &websiteService, log)
		}

		// Delete HPA if it exists
		var websitesHPA kv3.HorizontalPodAutoscaler
		if err := r.Get(ctx, req.NamespacedName, &websitesHPA); err == nil {
			return r.RemoveHPA(ctx, &websitesHPA, log)
		}

		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If we have the website resource we need to ensure that the child resources are created as well.
	log.Info("Ensuring Deployment is created", "Website", req.NamespacedName)
	var websiteDeployment kv2.Deployment
	if err := r.Get(ctx, req.NamespacedName, &websiteDeployment); err != nil {
		log.Info("unable to fetch Deployment for website", "Website", req.NamespacedName)
		// Create a deployment
		return r.CreateDeployment(ctx, req, website, log)
	}
	// Ensure that at least minimum number of replicas are maintained
	if *(websiteDeployment.Spec.Replicas) < *(website.Spec.MinReplica) {
		// Update the deployment with the required replica
		websiteDeployment.Spec.Replicas = website.Spec.MinReplica
		if err := r.Update(ctx, &websiteDeployment); err != nil {
			log.Error(err, "unable to update the deployment for website", "Website", req.NamespacedName)
			return ctrl.Result{}, err
		}
	}

	// Ensure that the service is created for the website
	log.Info("Ensuring Service is created", "Website", req.NamespacedName)
	var websiteService kv1.Service
	if err := r.Get(ctx, req.NamespacedName, &websiteService); err != nil {
		log.Info("unable to fetch Deployment for website", "Website", req.NamespacedName)
		// Create the service
		return r.CreateService(ctx, req, website, log)
	}

	// Ensure that the service is created for the website
	log.Info("Ensuring HPA is created", "Website", req.NamespacedName)
	var websitesHPA kv3.HorizontalPodAutoscaler
	if err := r.Get(ctx, req.NamespacedName, &websitesHPA); err != nil {
		log.Info("unable to fetch HPA for website", "Website", req.NamespacedName)
		return r.CreateHPA(ctx, req, website, log)
	}

	return ctrl.Result{}, nil
}

// Update status of the website
func (r *WebsiteReconciler) UpdateStatus(ctx context.Context, req ctrl.Request, website batchv1.Website, deplmntWebsite *kv2.Deployment) {
	// Here we are only maintaining the current replica count for the status, more can be done.
	website.Status.CurrentReplicas = deplmntWebsite.Spec.Replicas
}

// CreateDeployment creates the deployment in the cluster.
func (r *WebsiteReconciler) CreateDeployment(ctx context.Context, req ctrl.Request, website batchv1.Website, log logr.Logger) (ctrl.Result, error) {
	var websiteDeployment *kv2.Deployment
	websiteDeployment = &kv2.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    website.ObjectMeta.Labels,
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: kv2.DeploymentSpec{
			Replicas: website.Spec.MinReplica,
			Selector: &metav1.LabelSelector{
				MatchLabels: website.ObjectMeta.Labels,
			},
			Template: kv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:    website.ObjectMeta.Labels,
					Name:      req.Name,
					Namespace: req.Namespace,
				},
				Spec: kv1.PodSpec{
					Containers: []kv1.Container{
						kv1.Container{
							Name:  website.Name,
							Image: website.Spec.Image,
							Ports: []kv1.ContainerPort{
								{
									ContainerPort: website.Spec.Port,
								},
							},
							ImagePullPolicy: kv1.PullIfNotPresent,
							Resources: kv1.ResourceRequirements{
								Requests: kv1.ResourceList{
									kv1.ResourceCPU: website.Spec.CPURequest,
								},
							},
						},
					},
				},
			},
		},
	}
	if err := r.Create(ctx, websiteDeployment); err != nil {
		log.Error(err, "unable to create website deployment for Website", "website", websiteDeployment)
		return ctrl.Result{}, err
	}

	log.V(1).Info("created website deployment for Website run", "websitePod", websiteDeployment)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// RemoveDeployment deletes deployment from the cluster
func (r *WebsiteReconciler) RemoveDeployment(ctx context.Context, deplmtToRemove *kv2.Deployment, log logr.Logger) (ctrl.Result, error) {
	name := deplmtToRemove.Name
	if err := r.Delete(ctx, deplmtToRemove); err != nil {
		log.Error(err, "unable to delete website deployment for Website", "website", deplmtToRemove.Name)
		return ctrl.Result{}, err
	}
	log.V(1).Info("Removed website deployment for Website run", "websitePod", name)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// CreateService creates the desired service in the cluster
func (r *WebsiteReconciler) CreateService(ctx context.Context, req ctrl.Request, website batchv1.Website, log logr.Logger) (ctrl.Result, error) {
	var websiteService *kv1.Service
	websiteService = &kv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    website.ObjectMeta.Labels,
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: kv1.ServiceSpec{
			Ports: []kv1.ServicePort{
				{
					Port: website.Spec.Port,
				},
			},
			Selector: website.ObjectMeta.Labels,
		},
	}
	if err := r.Create(ctx, websiteService); err != nil {
		log.Error(err, "unable to create website service for Website", "website", websiteService)
		return ctrl.Result{}, err
	}

	log.V(1).Info("created website service for Website run", "websitePod", websiteService)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// RemoveService deletes the service from the cluster.
func (r *WebsiteReconciler) RemoveService(ctx context.Context, serviceToRemove *kv1.Service, log logr.Logger) (ctrl.Result, error) {
	name := serviceToRemove.Name
	if err := r.Delete(ctx, serviceToRemove); err != nil {
		log.Error(err, "unable to delete website service for Website", "website", serviceToRemove.Name)
		return ctrl.Result{}, err
	}
	log.V(1).Info("Removed website service for Website run", "websitePod", name)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// CreateHPA creates a HPA in the cluster
func (r *WebsiteReconciler) CreateHPA(ctx context.Context, req ctrl.Request, website batchv1.Website, log logr.Logger) (ctrl.Result, error) {
	var websiteHPA *kv3.HorizontalPodAutoscaler
	websiteHPA = &kv3.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    website.ObjectMeta.Labels,
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: kv3.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: kv3.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       website.Name,
				APIVersion: "apps/v1",
			},
			MinReplicas: website.Spec.MinReplica,
			MaxReplicas: website.Spec.MaxReplica,
			Metrics: []kv3.MetricSpec{
				kv3.MetricSpec{
					Type: kv3.MetricSourceType("Resource"),
					Resource: &kv3.ResourceMetricSource{
						Name:                     kv1.ResourceCPU,
						TargetAverageUtilization: website.Spec.CPULimit,
					},
				},
			},
		},
	}
	if err := r.Create(ctx, websiteHPA); err != nil {
		log.Error(err, "unable to create HPA for Website", "websiteHPA", websiteHPA)
		return ctrl.Result{}, err
	}

	log.V(1).Info("created HPA for Website run", "websiteHPA", websiteHPA)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// RemoveHPA deletes the HPA from the cluster
func (r *WebsiteReconciler) RemoveHPA(ctx context.Context, hpaToRemove *kv3.HorizontalPodAutoscaler, log logr.Logger) (ctrl.Result, error) {
	name := hpaToRemove.Name
	if err := r.Delete(ctx, hpaToRemove); err != nil {
		log.Error(err, "unable to delete website HPA for Website", "website", hpaToRemove.Name)
		return ctrl.Result{}, err
	}
	log.V(1).Info("Removed website HPA for Website run", "websitePod", name)
	return ctrl.Result{}, nil
}

var (
	jobOwnerKey = ".metadata.controller"
	apiGVStr    = batchv1.GroupVersion.String()
)

func (r *WebsiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(&kv2.Deployment{}, jobOwnerKey, func(rawObj runtime.Object) []string {
		// grab the deployment object, extract the owner...
		deployment := rawObj.(*kv2.Deployment)
		owner := metav1.GetControllerOf(deployment)
		if owner == nil {
			return nil
		}
		// ...make sure it's a Website...
		if owner.APIVersion != apiGVStr || owner.Kind != "Website" {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1.Website{}).
		Owns(&kv2.Deployment{}).
		Complete(r)
}
