package evaluation

// Counts use set membership; duplicate labels cannot inflate scores.
type LabelMetric struct {
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}
type Metrics struct {
	Samples          int                    `json:"samples"`
	Micro            LabelMetric            `json:"micro"`
	MacroF1          float64                `json:"macro_f1_all_taxonomy_labels"`
	MacroF1Supported float64                `json:"macro_f1_observed_labels"`
	ExactMatch       float64                `json:"exact_match"`
	PerLabel         map[string]LabelMetric `json:"per_label"`
	ZeroDivision     string                 `json:"zero_division_policy"`
}
type Pair struct{ Gold, Predicted []string }

func ratios(m LabelMetric) LabelMetric {
	if m.TP+m.FP > 0 {
		m.Precision = float64(m.TP) / float64(m.TP+m.FP)
	}
	if m.TP+m.FN > 0 {
		m.Recall = float64(m.TP) / float64(m.TP+m.FN)
	}
	if 2*m.TP+m.FP+m.FN > 0 {
		m.F1 = float64(2*m.TP) / float64(2*m.TP+m.FP+m.FN)
	}
	return m
}
func set(values []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range values {
		m[v] = true
	}
	return m
}
func Score(pairs []Pair, universe []string) *Metrics {
	if len(pairs) == 0 {
		return nil
	}
	m := &Metrics{Samples: len(pairs), PerLabel: map[string]LabelMetric{}, ZeroDivision: "undefined divisions are 0; no eligible samples yields null metrics"}
	for _, label := range universe {
		m.PerLabel[label] = LabelMetric{}
	}
	exact := 0
	for _, pair := range pairs {
		g, p := set(pair.Gold), set(pair.Predicted)
		same := len(g) == len(p)
		for label := range g {
			if !p[label] {
				same = false
			}
			v := m.PerLabel[label]
			if p[label] {
				v.TP++
			} else {
				v.FN++
			}
			m.PerLabel[label] = v
		}
		for label := range p {
			if !g[label] {
				v := m.PerLabel[label]
				v.FP++
				m.PerLabel[label] = v
			}
		}
		if same {
			exact++
		}
	}
	supported := 0
	for label, v := range m.PerLabel {
		v = ratios(v)
		m.PerLabel[label] = v
		m.Micro.TP += v.TP
		m.Micro.FP += v.FP
		m.Micro.FN += v.FN
		m.MacroF1 += v.F1
		if v.TP+v.FP+v.FN > 0 {
			m.MacroF1Supported += v.F1
			supported++
		}
	}
	m.Micro = ratios(m.Micro)
	if len(m.PerLabel) > 0 {
		m.MacroF1 /= float64(len(m.PerLabel))
	}
	if supported > 0 {
		m.MacroF1Supported /= float64(supported)
	}
	m.ExactMatch = float64(exact) / float64(len(pairs))
	return m
}
