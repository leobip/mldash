package api

import "testing"

func TestImageRepo(t *testing.T) {
	cases := []struct {
		image, want string
	}{
		{"mgmt-harbor.internal.dev.riss.space/aidev/orec-webservice:v1.4.1", "aidev/orec-webservice"},
		{"ghcr.io/leobip/mldash:0.4.0", "leobip/mldash"},
		{"registry.example.com:5000/team/svc:latest", "team/svc"},
		{"nginx", "nginx"},
		{"nginx:1.25", "nginx"},
		{"nvcr-proxy/nvidia/tritonserver:26.08-py3", "nvcr-proxy/nvidia/tritonserver"},
		{"ghcr.io/leobip/mldash@sha256:b7b93245", "leobip/mldash"},
		{"", ""},
	}
	for _, c := range cases {
		if got := ImageRepo(c.image); got != c.want {
			t.Errorf("ImageRepo(%q) = %q, want %q", c.image, got, c.want)
		}
	}
}

func TestImageTag(t *testing.T) {
	cases := []struct {
		image, want string
	}{
		{"mgmt-harbor.internal.dev.riss.space/aidev/orec-webservice:v1.4.1", "v1.4.1"},
		{"registry.example.com:5000/team/svc:latest", "latest"},
		{"registry.example.com:5000/team/svc", ""},
		{"nginx", ""},
		{"ghcr.io/leobip/mldash@sha256:b7b93245", "sha256:b7b93245"},
	}
	for _, c := range cases {
		if got := ImageTag(c.image); got != c.want {
			t.Errorf("ImageTag(%q) = %q, want %q", c.image, got, c.want)
		}
	}
}

func TestPodBaseAndSuffix(t *testing.T) {
	cases := []struct {
		name, wantBase, wantSuffix string
	}{
		{"orec-webservice-l4-6b57c886f5-6xcbs", "orec-webservice-l4", "6xcbs"},
		{"llm-code-795589c8-4hszr", "llm-code", "4hszr"},
		{"h100-p5s-reservation-859869884b-rrlfx", "h100-p5s-reservation", "rrlfx"},
		{"tesy-thom-0", "tesy-thom-0", ""}, // statefulset-style notebook pod
		{"open-webui-78fbbb47fc-vb2cf", "open-webui", "vb2cf"},
	}
	for _, c := range cases {
		if got := PodBase(c.name); got != c.wantBase {
			t.Errorf("PodBase(%q) = %q, want %q", c.name, got, c.wantBase)
		}
		if got := PodSuffix(c.name); got != c.wantSuffix {
			t.Errorf("PodSuffix(%q) = %q, want %q", c.name, got, c.wantSuffix)
		}
	}
}
