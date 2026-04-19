package kube

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var taskGVR = schema.GroupVersionResource{
	Group:    "vinculum.dev",
	Version:  "v1alpha1",
	Resource: "tasks",
}

type Client struct {
	dyn       dynamic.Interface
	namespace string
}

func NewInCluster(namespace string) (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	return &Client{dyn: dyn, namespace: namespace}, nil
}

func (c *Client) GetTask(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return c.dyn.Resource(taskGVR).Namespace(c.namespace).Get(ctx, name, metaGetOptions())
}

// PatchStatus applies a JSON merge patch against the /status subresource.
func (c *Client) PatchStatus(ctx context.Context, name string, status map[string]any) error {
	payload, err := json.Marshal(map[string]any{"status": status})
	if err != nil {
		return err
	}
	_, err = c.dyn.Resource(taskGVR).Namespace(c.namespace).Patch(
		ctx, name, types.MergePatchType, payload, metaPatchOptions(), "status",
	)
	return err
}
