package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/spf13/cobra"

	"github.com/florian/vinculum/apps/vnclm/internal/theme"
)

// Provider types and the env var keys they populate into the Secret.
var providerTypes = map[string][]string{
	"azure":     {"AZURE_OPENAI_API_ENDPOINT", "AZURE_OPENAI_API_KEY", "AZURE_OPENAI_API_VERSION"},
	"anthropic": {"ANTHROPIC_API_KEY"},
	"openai":    {"OPENAI_API_KEY"},
	"custom":    nil,
}

func createProviderCmd() *cobra.Command {
	var name, ptype string
	setFlag := map[string]string{}
	c := &cobra.Command{
		Use:   "provider",
		Short: "Create a provider Secret (wizard by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			kc, err := loadKube()
			if err != nil {
				return err
			}

			values := map[string]string{}
			for k, v := range setFlag {
				values[k] = v
			}

			wizard := name == "" || ptype == ""
			if wizard {
				if err := runProviderWizard(&name, &ptype, values); err != nil {
					return err
				}
			} else if ptype != "custom" {
				// Non-wizard mode: prompt for any well-known keys not supplied by --set.
				missing := []string{}
				for _, k := range providerTypes[ptype] {
					if _, ok := values[k]; !ok {
						missing = append(missing, k)
					}
				}
				if len(missing) > 0 {
					if err := runKeyForm(missing, values); err != nil {
						return err
					}
				}
			}

			if _, ok := providerTypes[ptype]; !ok {
				return errf("unknown provider type %q", ptype)
			}

			data := map[string][]byte{}
			for k, v := range values {
				if strings.TrimSpace(v) == "" {
					continue
				}
				data[k] = []byte(v)
			}
			if len(data) == 0 {
				return errf("no values provided")
			}
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: kc.Namespace,
					Labels:    map[string]string{"vinculum.dev/provider": "true"},
					Annotations: map[string]string{
						"vinculum.dev/provider-type": ptype,
					},
				},
				Type: corev1.SecretTypeOpaque,
				Data: data,
			}
			if _, err := kc.Core.CoreV1().Secrets(kc.Namespace).Create(ctx, sec, metav1.CreateOptions{}); err != nil {
				return err
			}
			fmt.Printf("provider %q created (type=%s, keys=%v)\n", name, ptype, keysOf(data))
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "provider name")
	c.Flags().StringVar(&ptype, "type", "", "provider type (azure|anthropic|openai|custom)")
	c.Flags().StringToStringVar(&setFlag, "set", nil, "key=value pairs to store in Secret")
	_ = c.RegisterFlagCompletionFunc("type", staticValues("azure", "anthropic", "openai", "custom"))
	return c
}

func runProviderWizard(name, ptype *string, values map[string]string) error {
	typeOptions := []huh.Option[string]{
		huh.NewOption("Azure OpenAI / AI Foundry", "azure"),
		huh.NewOption("Anthropic", "anthropic"),
		huh.NewOption("OpenAI", "openai"),
		huh.NewOption("Custom (arbitrary env vars)", "custom"),
	}
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Name").Value(name).Validate(nonEmpty),
			huh.NewSelect[string]().Title("Type").Options(typeOptions...).Value(ptype),
		),
	).WithTheme(theme.Huh()).Run(); err != nil {
		return err
	}
	keys := providerTypes[*ptype]
	if *ptype == "custom" {
		return nil
	}
	return runKeyForm(keys, values)
}

func runKeyForm(keys []string, values map[string]string) error {
	locals := make([]string, len(keys))
	fields := make([]huh.Field, 0, len(keys))
	for i, k := range keys {
		in := huh.NewInput().Title(k).Value(&locals[i])
		if strings.Contains(k, "KEY") || strings.Contains(k, "SECRET") {
			in = in.Password(true)
		}
		fields = append(fields, in)
	}
	if err := huh.NewForm(huh.NewGroup(fields...)).WithTheme(theme.Huh()).Run(); err != nil {
		return err
	}
	for i, k := range keys {
		values[k] = locals[i]
	}
	return nil
}

func keysOf(data map[string][]byte) []string {
	out := make([]string, 0, len(data))
	for k := range data {
		out = append(out, k)
	}
	return out
}

func nonEmpty(s string) error {
	if strings.TrimSpace(s) == "" {
		return errf("required")
	}
	return nil
}
