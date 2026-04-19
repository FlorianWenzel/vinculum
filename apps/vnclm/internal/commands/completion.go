package commands

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	gvrAgents    = schema.GroupVersionResource{Group: "vinculum.dev", Version: "v1alpha1", Resource: "agents"}
	gvrTasks     = schema.GroupVersionResource{Group: "vinculum.dev", Version: "v1alpha1", Resource: "tasks"}
	gvrSchedules = schema.GroupVersionResource{Group: "vinculum.dev", Version: "v1alpha1", Resource: "agentschedules"}
)

func completeResource(gvr schema.GroupVersionResource) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		kc, err := loadKube()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		list, err := kc.Dynamic.Resource(gvr).Namespace(kc.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		out := make([]string, 0, len(list.Items))
		for _, it := range list.Items {
			out = append(out, it.GetName())
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}

func completeProviders() cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		kc, err := loadKube()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		secrets, err := kc.Core.CoreV1().Secrets(kc.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "vinculum.dev/provider=true",
		})
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		out := make([]string, 0, len(secrets.Items))
		for _, s := range secrets.Items {
			out = append(out, s.Name)
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}

func staticValues(vals ...string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return vals, cobra.ShellCompDirectiveNoFileComp
	}
}
