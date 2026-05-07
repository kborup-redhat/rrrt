package collector

import (
	"fmt"
	"regexp"
	"strings"
)

// SanitizeLabelValue escapes special characters in a Prometheus label value
// for use in exact-match (=) selectors.
func SanitizeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// SanitizeRegexValue escapes a string for safe use in a Prometheus regex-match (=~) selector.
func SanitizeRegexValue(s string) string {
	return regexp.QuoteMeta(SanitizeLabelValue(s))
}

func vmCPUQuery(vmName, namespace, lookback string) string {
	return fmt.Sprintf(
		`rate(kubevirt_vmi_cpu_usage_seconds_total{name="%s",namespace="%s"}[5m])[%s:1m]`,
		SanitizeLabelValue(vmName), SanitizeLabelValue(namespace), lookback,
	)
}

func vmMemoryQuery(vmName, namespace, lookback string) string {
	return fmt.Sprintf(
		`kubevirt_vmi_memory_resident_bytes{name="%s",namespace="%s"}[%s]`,
		SanitizeLabelValue(vmName), SanitizeLabelValue(namespace), lookback,
	)
}

func containerCPUQuery(workloadName, namespace, lookback string) string {
	return fmt.Sprintf(
		`rate(container_cpu_usage_seconds_total{namespace="%s",pod=~"%s-.*",container!=""}[5m])[%s:1m]`,
		SanitizeLabelValue(namespace), SanitizeRegexValue(workloadName), lookback,
	)
}

func containerMemoryQuery(workloadName, namespace, lookback string) string {
	return fmt.Sprintf(
		`container_memory_working_set_bytes{namespace="%s",pod=~"%s-.*",container!=""}[%s]`,
		SanitizeLabelValue(namespace), SanitizeRegexValue(workloadName), lookback,
	)
}
