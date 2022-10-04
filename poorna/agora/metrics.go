package agora

import (
	"gitlab.com/sarvalabs/moichain/poorna/agora/decision"
	"gitlab.com/sarvalabs/moichain/poorna/agora/network"
)

type Metrics struct {
	Engine  *decision.Metrics
	Network *network.Metrics
}

func GetPrometheusMetrics(namespace string, labelsWithValues ...string) *Metrics {
	labels := make([]string, 0)

	for i := 0; i < len(labelsWithValues); i += 2 {
		labels = append(labels, labelsWithValues[i])
	}

	return &Metrics{
		Engine:  decision.GetPrometheusMetrics(namespace, labels, labelsWithValues...),
		Network: network.GetPrometheusMetrics(namespace, labels, labelsWithValues...),
	}
}

func NilMetrics() *Metrics {
	return &Metrics{
		Engine:  decision.NilMetrics(),
		Network: network.NilMetrics(),
	}
}
