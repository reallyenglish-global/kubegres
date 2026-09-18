package statefulset

import "testing"

func TestStatefulSetWrapperIsReadyForFailover(t *testing.T) {
	tests := []struct {
		name      string
		setReady  bool
		podReady  bool
		disrupted bool
		want      bool
	}{
		{name: "ready statefulset and pod", setReady: true, podReady: true, want: true},
		{name: "statefulset not ready", setReady: false, podReady: true, want: false},
		{name: "pod not ready", setReady: true, podReady: false, want: false},
		{name: "pod being disrupted", setReady: true, podReady: true, disrupted: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wrapper := StatefulSetWrapper{
				IsReady: test.setReady,
				Pod: PodWrapper{
					IsReady:                     test.podReady,
					IsBeingVoluntarilyDisrupted: test.disrupted,
				},
			}
			if got := wrapper.IsReadyForFailover(); got != test.want {
				t.Fatalf("IsReadyForFailover() = %v, want %v", got, test.want)
			}
		})
	}
}
