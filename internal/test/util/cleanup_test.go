package util

import (
	"context"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type cleanupClient struct {
	client.Client
	deleted bool
}

func (c *cleanupClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.deleted {
		return apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, key.Name)
	}
	return nil
}

func TestWaitForDeletedEmpty(t *testing.T) {
	if err := waitForDeleted(context.Background(), &cleanupClient{}, nil, time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestWaitForDeleted(t *testing.T) {
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "test"}}
	c := &cleanupClient{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := waitForDeleted(ctx, c, []client.Object{pod}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("expected named timeout, got %v", err)
	}
	c.deleted = true
	if err := waitForDeleted(context.Background(), c, []client.Object{pod}, time.Millisecond); err != nil {
		t.Fatal(err)
	}
}
