package controller

import (
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"regexp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"

	webappv1 "github.com/cyberdotgent/kube-webapp-operator/api/v1"
)

type WebAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=webapp.cyber.gent,resources=webapps,verbs=get;list;watch
// +kubebuilder:rbac:groups=webapp.cyber.gent,resources=webapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=webapp.cyber.gent,resources=webapps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=traefik.io,resources=ingressroutes,verbs=get;list;watch;create;update;patch;delete

func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var app webappv1.WebApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if app.ObjectMeta.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	labels := labelsForWebApp(&app)

	for _, volume := range app.Spec.Volumes {
		pvcName := pvcNameForVolume(&app, volume)

		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: app.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
			size := volume.Size
			if size == "" {
				size = "10Gi"
			}

			quantity, err := resource.ParseQuantity(size)
			if err != nil {
				return fmt.Errorf("invalid volume size %q for mountPath %q: %w", size, volume.MountPath, err)
			}

			pvc.Labels = mergeStringMap(pvc.Labels, labels)
			pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			}
			pvc.Spec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceStorage: quantity,
			}

			return controllerutil.SetControllerReference(&app, pvc, r.Scheme)
		})
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		deployment.Labels = mergeStringMap(deployment.Labels, labels)

		replicas := int32(1)

		deployment.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:         "web",
							Image:        app.Spec.Image,
							Ports:        containerPortsForWebApp(&app),
							Env:          envVarsForWebApp(&app),
							VolumeMounts: volumeMountsForWebApp(&app),
						},
					},
					Volumes: podVolumesForWebApp(&app),
				},
			},
		}

		return controllerutil.SetControllerReference(&app, deployment, r.Scheme)
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		service.Labels = mergeStringMap(service.Labels, labels)
		service.Spec.Selector = labels
		service.Spec.Type = corev1.ServiceTypeClusterIP
		service.Spec.Ports = []corev1.ServicePort{
			{
				Name:       protoForWebApp(&app),
				Port:       app.Spec.Port,
				TargetPort: intstrFromInt32(app.Spec.Port),
				Protocol:   corev1.ProtocolTCP,
			},
		}

		return controllerutil.SetControllerReference(&app, service, r.Scheme)
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	if strings.TrimSpace(app.Spec.Ingress) == "" {
		if err := r.deleteIngressRoute(ctx, &app); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if err := r.reconcileIngressRoute(ctx, &app, labels); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.Info("reconciled WebApp", "name", app.Name, "namespace", app.Namespace)

	return ctrl.Result{}, nil
}

func (r *WebAppReconciler) reconcileIngressRoute(ctx context.Context, app *webappv1.WebApp, labels map[string]string) error {
	ingressRoute := &unstructured.Unstructured{}
	ingressRoute.SetAPIVersion("traefik.io/v1alpha1")
	ingressRoute.SetKind("IngressRoute")
	ingressRoute.SetName(app.Name + "-frontend")
	ingressRoute.SetNamespace(app.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ingressRoute, func() error {
		ingressRoute.SetLabels(mergeStringMap(ingressRoute.GetLabels(), labels))

		service := map[string]interface{}{
			"name": app.Name,
			"port": app.Spec.Port,
		}

		if protoForWebApp(app) == "https" {
			service["scheme"] = "https"
		}

		ingressRoute.Object["spec"] = map[string]interface{}{
			"routes": []interface{}{
				map[string]interface{}{
					"match": fmt.Sprintf("Host(`%s`)", app.Spec.Ingress),
					"kind":  "Rule",
					"services": []interface{}{
						service,
					},
				},
			},
		}

		return controllerutil.SetControllerReference(app, ingressRoute, r.Scheme)
	})

	return err
}

func (r *WebAppReconciler) deleteIngressRoute(ctx context.Context, app *webappv1.WebApp) error {
	ingressRoute := &unstructured.Unstructured{}
	ingressRoute.SetAPIVersion("traefik.io/v1alpha1")
	ingressRoute.SetKind("IngressRoute")
	ingressRoute.SetName(app.Name + "-frontend")
	ingressRoute.SetNamespace(app.Namespace)

	err := r.Delete(ctx, ingressRoute)
	if apierrors.IsNotFound(err) {
		return nil
	}

	return err
}

func labelsForWebApp(app *webappv1.WebApp) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       app.Name,
		"app.kubernetes.io/managed-by": "kube-webapp-operator",
		"app.kubernetes.io/part-of":    "webapp",
	}
}

func protoForWebApp(app *webappv1.WebApp) string {
	switch strings.ToLower(strings.TrimSpace(app.Spec.Proto)) {
	case "https":
		return "https"
	default:
		return "http"
	}
}

func containerPortsForWebApp(app *webappv1.WebApp) []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          protoForWebApp(app),
			ContainerPort: app.Spec.Port,
			Protocol:      corev1.ProtocolTCP,
		},
	}
}

func envVarsForWebApp(app *webappv1.WebApp) []corev1.EnvVar {
	envVars := make([]corev1.EnvVar, 0, len(app.Spec.Env))

	for _, env := range app.Spec.Env {
		if env.FromSecret != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name: env.Name,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: env.FromSecret,
						},
						Key: env.Name,
					},
				},
			})
			continue
		}

		envVars = append(envVars, corev1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
	}

	return envVars
}

func volumeMountsForWebApp(app *webappv1.WebApp) []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, 0, len(app.Spec.Volumes))

	for _, volume := range app.Spec.Volumes {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      podVolumeNameForVolume(app, volume),
			MountPath: volume.MountPath,
		})
	}

	return mounts
}

func podVolumesForWebApp(app *webappv1.WebApp) []corev1.Volume {
	volumes := make([]corev1.Volume, 0, len(app.Spec.Volumes))

	for _, volume := range app.Spec.Volumes {
		volumes = append(volumes, corev1.Volume{
			Name: podVolumeNameForVolume(app, volume),
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcNameForVolume(app, volume),
				},
			},
		})
	}

	return volumes
}

func podVolumeNameForVolume(app *webappv1.WebApp, volume webappv1.VolumeSpec) string {
	if strings.TrimSpace(volume.Name) != "" {
		return sanitizeDNS1123Label(volume.Name)
	}

	return sanitizeDNS1123Label(app.Name + "-" + strings.Trim(volume.MountPath, "/"))
}

func pvcNameForVolume(app *webappv1.WebApp, volume webappv1.VolumeSpec) string {
	return sanitizeDNS1123Label(app.Name + "-" + podVolumeNameForVolume(app, volume))
}

func sanitizeDNS1123Label(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, ".", "-")

	re := regexp.MustCompile(`[^a-z0-9-]+`)
	value = re.ReplaceAllString(value, "-")

	value = strings.Trim(value, "-")

	if value == "" {
		value = "webapp-volume"
	}

	if len(value) > 63 {
		value = value[:63]
		value = strings.TrimRight(value, "-")
	}

	return value
}

func mergeStringMap(existing map[string]string, desired map[string]string) map[string]string {
	out := map[string]string{}

	for k, v := range existing {
		out[k] = v
	}

	for k, v := range desired {
		out[k] = v
	}

	return out
}

func intstrFromInt32(value int32) intstr.IntOrString {
	return intstr.FromInt(int(value))
}

func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1.WebApp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Named("webapp").
		Complete(r)
}
