package resources_count_spec

import (
	"crypto/sha256"
	"fmt"

	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	postgresV1 "reactive-tech.io/kubegres/api/v1"
	"reactive-tech.io/kubegres/internal/controller/ctx"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	cronTaskManagedByLabel = "kubegres.reactive-tech.io/cron-task"
	cronTaskNameLabel      = "kubegres.reactive-tech.io/cron-task-name"
	cronTaskContainerName  = "kubegres-task"
	cronTaskScriptVolume   = "kubegres-task-script"
)

// CronTasksCountSpecEnforcer reconciles the independent CronJobs declared in
// spec.cronTasks. Backup remains managed by BackUpCronJobCountSpecEnforcer.
type CronTasksCountSpecEnforcer struct {
	kubegresContext ctx.KubegresContext
}

func CreateCronTasksCountSpecEnforcer(kubegresContext ctx.KubegresContext) CronTasksCountSpecEnforcer {
	return CronTasksCountSpecEnforcer{kubegresContext: kubegresContext}
}

func (r *CronTasksCountSpecEnforcer) EnforceSpec() error {
	wanted := make(map[string]postgresV1.KubegresCronTask, len(r.kubegresContext.Kubegres.Spec.CronTasks))
	for _, task := range r.kubegresContext.Kubegres.Spec.CronTasks {
		wanted[cronTaskName(r.kubegresContext.Kubegres.Name, task.Name)] = task
	}

	var deployed batch.CronJobList
	if err := r.kubegresContext.Client.List(r.kubegresContext.Ctx, &deployed, client.InNamespace(r.kubegresContext.Kubegres.Namespace)); err != nil {
		return err
	}
	for i := range deployed.Items {
		cronJob := &deployed.Items[i]
		if !r.isManagedCronTask(cronJob) {
			continue
		}
		if _, ok := wanted[cronJob.Name]; !ok {
			if err := r.kubegresContext.Client.Delete(r.kubegresContext.Ctx, cronJob); err != nil {
				return err
			}
		}
	}

	for _, task := range r.kubegresContext.Kubegres.Spec.CronTasks {
		if err := r.reconcileTask(task); err != nil {
			return err
		}
	}
	return nil
}

func (r *CronTasksCountSpecEnforcer) reconcileTask(task postgresV1.KubegresCronTask) error {
	desired := r.newCronJob(task)
	current := &batch.CronJob{}
	key := client.ObjectKey{Namespace: desired.Namespace, Name: desired.Name}
	if err := r.kubegresContext.Client.Get(r.kubegresContext.Ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return r.kubegresContext.Client.Create(r.kubegresContext.Ctx, desired)
		}
		return err
	}
	if !r.isManagedCronTask(current) {
		return fmt.Errorf("CronJob %q already exists but is not managed by this Kubegres cron task", current.Name)
	}
	desired.ResourceVersion = current.ResourceVersion
	return r.kubegresContext.Client.Update(r.kubegresContext.Ctx, desired)
}

func (r *CronTasksCountSpecEnforcer) newCronJob(task postgresV1.KubegresCronTask) *batch.CronJob {
	postgres := r.kubegresContext.Kubegres
	container := core.Container{
		Name:         cronTaskContainerName,
		Image:        task.Image,
		Command:      task.Command,
		Args:         task.Args,
		Env:          mergedEnv(postgres.Spec.Env, task.Env),
		VolumeMounts: append([]core.VolumeMount{}, task.VolumeMounts...),
	}
	podSpec := core.PodSpec{
		RestartPolicy:    core.RestartPolicyOnFailure,
		Containers:       []core.Container{container},
		Volumes:          append([]core.Volume{}, task.Volumes...),
		ImagePullSecrets: append([]core.LocalObjectReference{}, postgres.Spec.ImagePullSecrets...),
	}
	if task.Script != nil {
		podSpec.Volumes = append(podSpec.Volumes, core.Volume{
			Name: cronTaskScriptVolume,
			VolumeSource: core.VolumeSource{ConfigMap: &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{Name: task.Script.ConfigMapName},
			}},
		})
		podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, core.VolumeMount{
			Name: cronTaskScriptVolume, MountPath: task.Script.MountPath, SubPath: task.Script.Key,
		})
	}

	cronJob := &batch.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cronTaskName(postgres.Name, task.Name),
			Namespace:   postgres.Namespace,
			Annotations: customAnnotations(postgres.Annotations),
			Labels: map[string]string{
				cronTaskManagedByLabel: "true",
				cronTaskNameLabel:      task.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(postgres, postgresV1.GroupVersion.WithKind(ctx.KindKubegres))},
		},
		Spec: batch.CronJobSpec{
			Schedule: task.Schedule,
			JobTemplate: batch.JobTemplateSpec{Spec: batch.JobSpec{Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: customAnnotations(postgres.Annotations)},
				Spec:       podSpec,
			}}},
		},
	}
	if task.ConcurrencyPolicy != "" {
		cronJob.Spec.ConcurrencyPolicy = batch.ConcurrencyPolicy(task.ConcurrencyPolicy)
	}
	cronJob.Spec.SuccessfulJobsHistoryLimit = task.SuccessfulJobsHistoryLimit
	cronJob.Spec.FailedJobsHistoryLimit = task.FailedJobsHistoryLimit
	return cronJob
}

func (r *CronTasksCountSpecEnforcer) isManagedCronTask(cronJob *batch.CronJob) bool {
	if cronJob.Labels[cronTaskManagedByLabel] != "true" {
		return false
	}
	for _, owner := range cronJob.OwnerReferences {
		if owner.UID == r.kubegresContext.Kubegres.UID && owner.Controller != nil && *owner.Controller {
			return true
		}
	}
	return false
}

func cronTaskName(kubegresName, taskName string) string {
	name := fmt.Sprintf("%s-task-%s", kubegresName, taskName)
	if len(name) <= 63 {
		return name
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(name)))[:8]
	return name[:54] + "-" + digest
}

func mergedEnv(inherited, local []core.EnvVar) []core.EnvVar {
	localNames := make(map[string]struct{}, len(local))
	for _, env := range local {
		localNames[env.Name] = struct{}{}
	}
	merged := make([]core.EnvVar, 0, len(inherited)+len(local))
	for _, env := range inherited {
		if _, overridden := localNames[env.Name]; !overridden {
			merged = append(merged, env)
		}
	}
	return append(merged, local...)
}

func customAnnotations(annotations map[string]string) map[string]string {
	result := make(map[string]string, len(annotations))
	for key, value := range annotations {
		if key != "kubectl.kubernetes.io/last-applied-configuration" {
			result[key] = value
		}
	}
	return result
}
