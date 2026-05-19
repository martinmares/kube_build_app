package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	Kubeconfig string
	Context    string
	Timeout    time.Duration
}

type Summary struct {
	Namespace        string             `json:"namespace"`
	Available        bool               `json:"available"`
	Error            string             `json:"error,omitempty"`
	DeploymentCount  int                `json:"deployment_count"`
	ReadyDeployments int                `json:"ready_deployments"`
	PodCount         int                `json:"pod_count"`
	ReadyPods        int                `json:"ready_pods"`
	ServiceCount     int                `json:"service_count"`
	Deployments      []DeploymentStatus `json:"deployments"`
	Pods             []PodStatus        `json:"pods"`
	Services         []ServiceStatus    `json:"services"`
}

type DeploymentStatus struct {
	Name      string `json:"name"`
	Desired   int32  `json:"desired"`
	Ready     int32  `json:"ready"`
	Available int32  `json:"available"`
	Updated   int32  `json:"updated"`
}

type PodStatus struct {
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Ready    string `json:"ready"`
	Restarts int32  `json:"restarts"`
}

type ServiceStatus struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	ClusterIP string `json:"cluster_ip"`
	Ports     string `json:"ports"`
}

func InspectNamespace(ctx context.Context, namespace string, opts Options) (Summary, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return Summary{}, errors.New("namespace is required")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	deployments, err := getDeployments(ctx, namespace, opts)
	if err != nil {
		return Summary{Namespace: namespace, Available: false, Error: err.Error()}, nil
	}
	pods, err := getPods(ctx, namespace, opts)
	if err != nil {
		return Summary{Namespace: namespace, Available: false, Error: err.Error()}, nil
	}
	services, err := getServices(ctx, namespace, opts)
	if err != nil {
		return Summary{Namespace: namespace, Available: false, Error: err.Error()}, nil
	}
	result := Summary{Namespace: namespace, Available: true, Deployments: deployments, Pods: pods, Services: services, DeploymentCount: len(deployments), PodCount: len(pods), ServiceCount: len(services)}
	for _, deployment := range deployments {
		if deployment.Desired == deployment.Ready && deployment.Desired == deployment.Available {
			result.ReadyDeployments++
		}
	}
	for _, pod := range pods {
		if pod.Phase == "Running" && strings.HasPrefix(pod.Ready, readyTotal(pod.Ready)+"/") {
			result.ReadyPods++
		}
	}
	return result, nil
}

func getDeployments(ctx context.Context, namespace string, opts Options) ([]DeploymentStatus, error) {
	var payload deploymentList
	if err := kubectlJSON(ctx, namespace, opts, "deployments", &payload); err != nil {
		return nil, err
	}
	result := make([]DeploymentStatus, 0, len(payload.Items))
	for _, item := range payload.Items {
		result = append(result, DeploymentStatus{Name: item.Metadata.Name, Desired: item.Spec.Replicas, Ready: item.Status.ReadyReplicas, Available: item.Status.AvailableReplicas, Updated: item.Status.UpdatedReplicas})
	}
	return result, nil
}

func getPods(ctx context.Context, namespace string, opts Options) ([]PodStatus, error) {
	var payload podList
	if err := kubectlJSON(ctx, namespace, opts, "pods", &payload); err != nil {
		return nil, err
	}
	result := make([]PodStatus, 0, len(payload.Items))
	for _, item := range payload.Items {
		ready := int32(0)
		restarts := int32(0)
		for _, container := range item.Status.ContainerStatuses {
			if container.Ready {
				ready++
			}
			restarts += container.RestartCount
		}
		result = append(result, PodStatus{Name: item.Metadata.Name, Phase: item.Status.Phase, Ready: fmt.Sprintf("%d/%d", ready, len(item.Status.ContainerStatuses)), Restarts: restarts})
	}
	return result, nil
}

func getServices(ctx context.Context, namespace string, opts Options) ([]ServiceStatus, error) {
	var payload serviceList
	if err := kubectlJSON(ctx, namespace, opts, "services", &payload); err != nil {
		return nil, err
	}
	result := make([]ServiceStatus, 0, len(payload.Items))
	for _, item := range payload.Items {
		ports := make([]string, 0, len(item.Spec.Ports))
		for _, port := range item.Spec.Ports {
			text := strconv.Itoa(int(port.Port))
			if targetPort := formatTargetPort(port.TargetPort); targetPort != "" {
				text += ":" + targetPort
			}
			if port.Protocol != "" {
				text += "/" + port.Protocol
			}
			ports = append(ports, text)
		}
		result = append(result, ServiceStatus{Name: item.Metadata.Name, Type: item.Spec.Type, ClusterIP: item.Spec.ClusterIP, Ports: strings.Join(ports, ", ")})
	}
	return result, nil
}

func formatTargetPort(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return strconv.Itoa(number)
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return ""
}

func kubectlJSON(ctx context.Context, namespace string, opts Options, resource string, target any) error {
	args := []string{}
	if strings.TrimSpace(opts.Kubeconfig) != "" {
		args = append(args, "--kubeconfig", opts.Kubeconfig)
	}
	if strings.TrimSpace(opts.Context) != "" {
		args = append(args, "--context", opts.Context)
	}
	args = append(args, "-n", namespace, "get", resource, "-o", "json")
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return fmt.Errorf("kubectl get %s: %w", resource, err)
		}
		return fmt.Errorf("kubectl get %s: %w: %s", resource, err, text)
	}
	if err := json.Unmarshal(out, target); err != nil {
		return fmt.Errorf("parse kubectl %s JSON: %w", resource, err)
	}
	return nil
}

func readyTotal(value string) string {
	_, total, ok := strings.Cut(value, "/")
	if !ok {
		return ""
	}
	return total
}

type objectMeta struct {
	Name string `json:"name"`
}

type deploymentList struct {
	Items []struct {
		Metadata objectMeta `json:"metadata"`
		Spec     struct {
			Replicas int32 `json:"replicas"`
		} `json:"spec"`
		Status struct {
			ReadyReplicas     int32 `json:"readyReplicas"`
			AvailableReplicas int32 `json:"availableReplicas"`
			UpdatedReplicas   int32 `json:"updatedReplicas"`
		} `json:"status"`
	} `json:"items"`
}

type podList struct {
	Items []struct {
		Metadata objectMeta `json:"metadata"`
		Status   struct {
			Phase             string `json:"phase"`
			ContainerStatuses []struct {
				Ready        bool  `json:"ready"`
				RestartCount int32 `json:"restartCount"`
			} `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

type serviceList struct {
	Items []struct {
		Metadata objectMeta `json:"metadata"`
		Spec     struct {
			Type      string `json:"type"`
			ClusterIP string `json:"clusterIP"`
			Ports     []struct {
				Port       int32           `json:"port"`
				TargetPort json.RawMessage `json:"targetPort"`
				Protocol   string          `json:"protocol"`
			} `json:"ports"`
		} `json:"spec"`
	} `json:"items"`
}
