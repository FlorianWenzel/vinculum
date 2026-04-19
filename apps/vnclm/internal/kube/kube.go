package kube

import (
	"fmt"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client bundles the pieces of a kube client vnclm needs.
type Client struct {
	Config    *rest.Config
	Core      *kubernetes.Clientset
	Dynamic   dynamic.Interface
	Namespace string
	Context   string
}

func Load(contextOverride, namespaceOverride string) (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	if contextOverride != "" {
		overrides.CurrentContext = contextOverride
	}
	kc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restCfg, err := kc.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kube config: %w", err)
	}
	core, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	ns := namespaceOverride
	if ns == "" {
		ns, _, _ = kc.Namespace()
	}
	if ns == "" {
		ns = "default"
	}
	rawCfg, _ := kc.RawConfig()
	return &Client{
		Config:    restCfg,
		Core:      core,
		Dynamic:   dyn,
		Namespace: ns,
		Context:   rawCfg.CurrentContext,
	}, nil
}

// ConfigFlags builds a genericclioptions.ConfigFlags wiring in the usual
// kubeconfig resolution and the given namespace. Useful for port-forward.
func ConfigFlags(namespace string) *genericclioptions.ConfigFlags {
	f := genericclioptions.NewConfigFlags(true)
	if namespace != "" {
		f.Namespace = &namespace
	}
	return f
}
