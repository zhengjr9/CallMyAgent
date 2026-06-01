package main

import (
	"context"
	"fmt"
	"log"
	"os"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func getK8sClient() (*kubernetes.Clientset, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("HOME") + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build kubeconfig: %w", err)
		}
	}
	return kubernetes.NewForConfig(config)
}

func CreateClaudeJob(cfg *Config, task *Task) (string, error) {
	clientset, err := getK8sClient()
	if err != nil {
		return "", fmt.Errorf("k8s client: %w", err)
	}

	jobName := fmt.Sprintf("callmyagent-%s", task.ID)

	promptContent := taskPrompt(task)

	// Create ConfigMap for the prompt
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName + "-prompt",
			Namespace: cfg.K8sNamespace,
			Labels: map[string]string{
				"app":       "callmyagent",
				"task-id":   task.ID,
				"component": "worker",
			},
		},
		Data: map[string]string{
			"prompt.txt": promptContent,
		},
	}

	_, err = clientset.CoreV1().ConfigMaps(cfg.K8sNamespace).Create(
		context.TODO(), configMap, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("create configmap: %w", err)
	}

	// Build environment variables
	envVars := []corev1.EnvVar{
		{Name: "ANTHROPIC_API_KEY", ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "claude-api-secret"},
				Key:                  "api-key",
			},
		}},
		{Name: "CALLMYAGENT_TASK_ID", Value: task.ID},
		{Name: "GIT_REPO", Value: task.GitRepo},
		{Name: "GIT_BRANCH", Value: task.GitBranch},
		{Name: "TASK_ENGINE", Value: task.Engine},
	}
	envVars[0].ValueFrom.SecretKeyRef.Name = "callmyagent-api-secret"

	if cfg.AnthropicBaseURL != "" && cfg.AnthropicBaseURL != "https://api.anthropic.com" {
		envVars = append(envVars, corev1.EnvVar{Name: "ANTHROPIC_BASE_URL", Value: cfg.AnthropicBaseURL})
	}
	if cfg.CodexBaseURL != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "OPENAI_BASE_URL", Value: cfg.CodexBaseURL})
		envVars = append(envVars, corev1.EnvVar{Name: "CODEX_BASE_URL", Value: cfg.CodexBaseURL})
	}
	if cfg.CodexModel != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "OPENAI_MODEL", Value: cfg.CodexModel})
		envVars = append(envVars, corev1.EnvVar{Name: "CODEX_MODEL", Value: cfg.CodexModel})
	}
	if cfg.CodexAPIKey != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "OPENAI_API_KEY", Value: cfg.CodexAPIKey})
		envVars = append(envVars, corev1.EnvVar{Name: "CODEX_API_KEY", Value: cfg.CodexAPIKey})
	} else {
		envVars = append(envVars, corev1.EnvVar{Name: "OPENAI_API_KEY", ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "callmyagent-api-secret"},
				Key:                  "codex-api-key",
				Optional:             boolPtr(true),
			},
		}})
		envVars = append(envVars, corev1.EnvVar{Name: "CODEX_API_KEY", ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "callmyagent-api-secret"},
				Key:                  "codex-api-key",
				Optional:             boolPtr(true),
			},
		}})
	}

	if cfg.ClaudeAPIToken != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "CLAUDE_API_TOKEN",
			Value: cfg.ClaudeAPIToken,
		})
	}

	// Volume mounts
	volumes := []corev1.Volume{
		{
			Name: "prompt-volume",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: jobName + "-prompt",
					},
				},
			},
		},
		{
			Name: "work-volume",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(1<<30, resource.BinarySI), // 1Gi
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{Name: "prompt-volume", MountPath: "/prompt", ReadOnly: true},
		{Name: "work-volume", MountPath: "/workspace"},
	}

	// Add PVC if configured
	if cfg.OutputPVC != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "output-volume",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: cfg.OutputPVC,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name: "output-volume", MountPath: "/output",
		})
	}

	maxTurns, budgetUSD := taskLimits(task)

	// Job definition
	backoffLimit := int32(1)
	activeDeadlineSeconds := int64(1800) // 30 minutes
	ttlSeconds := int32(3600)            // 1 hour cleanup

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cfg.K8sNamespace,
			Labels: map[string]string{
				"app":       "callmyagent",
				"task-id":   task.ID,
				"component": "worker",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   &activeDeadlineSeconds,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "callmyagent",
						"task-id": task.ID,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "callmyagent-worker",
							Image:   cfg.ContainerImage,
							Command: []string{"/bin/bash", "/scripts/entrypoint.sh"},
							Env: append(envVars,
								corev1.EnvVar{Name: "MAX_TURNS", Value: fmt.Sprintf("%d", maxTurns)},
								corev1.EnvVar{Name: "BUDGET_USD", Value: fmt.Sprintf("%.2f", budgetUSD)},
							),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	_, err = clientset.BatchV1().Jobs(cfg.K8sNamespace).Create(
		context.TODO(), job, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("create job: %w", err)
	}

	log.Printf("Created K8s job: %s for task: %s", jobName, task.ID)
	return jobName, nil
}

func boolPtr(v bool) *bool {
	return &v
}

func GetJobStatus(cfg *Config, jobName string) (string, error) {
	clientset, err := getK8sClient()
	if err != nil {
		return "", err
	}

	job, err := clientset.BatchV1().Jobs(cfg.K8sNamespace).Get(
		context.TODO(), jobName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	if job.Status.Succeeded > 0 {
		return "completed", nil
	}
	if job.Status.Failed > 0 {
		return "failed", nil
	}
	if job.Status.Active > 0 {
		return "running", nil
	}
	return "pending", nil
}
