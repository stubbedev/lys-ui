package anim

import (
	"testing"
)

type pinnedBezier struct {
	points [4]float64
	want   []float64
}

type pinnedStep struct {
	omega, zeta, x0, v0, dt float64
	wantX, wantV            float64
}

var pinnedBeziers = []pinnedBezier{
	{[4]float64{0.25, 0.1, 0.25, 1}, []float64{0x1.8449218d3f7aep-04, 0x1.2e5496cdaeebbp-02, 0x1.06d1452cf8cb5p-01, 0x1.5d75f2ff0e457p-01, 0x1.9ad49de8b3eb8p-01, 0x1.c53cc6aee7fa9p-01, 0x1.e1abe64749942p-01, 0x1.f3852b18c1897p-01, 0x1.fd170ca18dfbfp-01}},
	{[4]float64{0.05, 0.7, 0.1, 1}, []float64{0x1.3e2618a4615d9p-01, 0x1.9192467e783a8p-01, 0x1.bc178c8223ba4p-01, 0x1.d5db1f71e67efp-01, 0x1.e686d64b3f228p-01, 0x1.f185a1b89f067p-01, 0x1.f8a96500dbcecp-01, 0x1.fd0744c008487p-01, 0x1.ff5131ff6c4cbp-01}},
	{[4]float64{0.3, 0, 0.8, 0.15}, []float64{0x1.6af463c183729p-08, 0x1.6f0068db8bac7p-06, 0x1.a60f7d5c2a46ap-05, 0x1.83445f1481f96p-04, 0x1.3b6367486f0fap-03, 0x1.de9d2f0507418p-03, 0x1.5bde8836c9417p-02, 0x1.ee1fe8f2979d9p-02, 0x1.5dd99e69e8fdep-01}},
	{[4]float64{0.2, 0, 0, 1}, []float64{0x1.3ffffffce97efp-03, 0x1.0000022f4942ap-01, 0x1.604a4fb1ddcd9p-01, 0x1.9ac4e9e10edd2p-01, 0x1.c17368063ba3ep-01, 0x1.dbaf556e6deabp-01, 0x1.ed3814cd94dd3p-01, 0x1.f83fd7b0d7ffap-01, 0x1.fe2fe1219148ap-01}},
	{[4]float64{0.68, -0.6, 0.32, 1.6}, []float64{-0x1.2a48a383249ecp-04, -0x1.ac7db3436623bp-04, -0x1.193ed53e8fed9p-04, 0x1.8c0ad4700e0e2p-04, 0x1p-01, 0x1.ce7ea571fe3e6p-01, 0x1.1193ed53e8ffp+00, 0x1.1ac7db3436624p+00, 0x1.12a48a383249dp+00}},
	{[4]float64{0, 1, 1, 0}, []float64{0x1.8cb2ab32a394cp-02, 0x1.d87f0f0db81b3p-02, 0x1.f586ea4cd5d03p-02, 0x1.fec3a74ae122ep-02, 0x1p-01, 0x1.009e2c5a8f6e8p-01, 0x1.053c8ad99517dp-01, 0x1.13c0787923f26p-01, 0x1.39a6aa66ae35ap-01}},
	{[4]float64{-0.5, 0, 1.5, 1}, []float64{0x1.999999a43fe2dp-04, 0x1.99999e082e9c5p-03, 0x1.33333333d6a1cp-02, 0x1.99999df38ae03p-02, 0x1p-01, 0x1.333331063a8ffp-01, 0x1.6666666614af3p-01, 0x1.9999987df458fp-01, 0x1.cccccccb7803cp-01}},
}

var pinnedSteps = []pinnedStep{
	{0x1.1f3b3855544c8p+04, 1, -1, 0, 0.016, -0x1.ee8509dbf7edfp-01, 0x1.ef3bf3d6cd4b5p+01},
	{0x1.1f3b3855544c8p+04, 1, -80, 40, 0.12, -0x1.cb666038c1656p+04, 0x1.6180aca851ef9p+08},
	{0x1.1f3b3855544c8p+04, 1, 300, -1500, 0.4, 0x1.6944c285a8497p+00, -0x1.6606ed08e455fp+04},
	{0x1.4f1a6c638d03fp+04, 0.86, -1, 0, 0.016, -0x1.e8471d62dc073p-01, 0x1.4f12c0c32a285p+02},
	{0x1.4f1a6c638d03fp+04, 0.86, -80, 40, 0.12, -0x1.1178cba21d034p+04, 0x1.646389dc394bbp+08},
	{0x1.4f1a6c638d03fp+04, 0.86, 300, -1500, 0.4, -0x1.5c70428a4e4adp-02, 0x1.c3bff8f3a3aap+02},
	{0x1.becde5da115a9p+03, 0.68, -1, 0, 0.016, -0x1.f478ca7d9d4fbp-01, 0x1.5576eef11f145p+01},
	{0x1.becde5da115a9p+03, 0.68, -80, 40, 0.12, -0x1.dc8302953b067p+04, 0x1.c45fe2ef4749dp+08},
	{0x1.becde5da115a9p+03, 0.68, 300, -1500, 0.4, -0x1.9346b079e8055p+02, 0x1.89e4994083e96p+06},
	{0x1.4f1a6c638d03fp+04, 1.6, -1, 0, 0.016, -0x1.eb73954cc5254p-01, 0x1.0e807a2ae70c6p+02},
	{0x1.4f1a6c638d03fp+04, 1.6, -80, 40, 0.12, -0x1.2b830131da567p+05, 0x1.12ccf59c10ba7p+08},
	{0x1.4f1a6c638d03fp+04, 1.6, 300, -1500, 0.4, 0x1.0904bc0dde2eap+04, -0x1.e70f58a2ff2d9p+06},
}

// The values CubicBezierAt gave before its solver moved to lys-lib's
// motion package.
func TestCubicBezierHoldsItsPinnedValues(t *testing.T) {
	for _, c := range pinnedBeziers {
		for i, want := range c.want {
			x := float64(i+1) / 10
			if got := CubicBezierAt(c.points[0], c.points[1], c.points[2], c.points[3], x); got != want {
				t.Errorf("%v at %v = %x, want %x", c.points, x, got, want)
			}
		}
	}
}

// The values springStep gave before its solver moved to lys-lib's
// motion package.
func TestSpringStepHoldsItsPinnedValues(t *testing.T) {
	for _, c := range pinnedSteps {
		if x, v := springStep(c.omega, c.zeta, c.x0, c.v0, c.dt); x != c.wantX || v != c.wantV {
			t.Errorf("%+v = (%x, %x)", c, x, v)
		}
	}
}
