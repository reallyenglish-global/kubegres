package util

import (
	"context"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

// waitForDeleted waits only for captured cleanup targets, not unrelated resources.
func waitForDeleted(ctx context.Context, c client.Client, targets []client.Object, interval time.Duration) error {
	for _, target := range targets {
		key := client.ObjectKeyFromObject(target)
		for {
			err := c.Get(ctx, key, target.DeepCopyObject().(client.Object))
			if apierrors.IsNotFound(err) {
				break
			}
			if err != nil {
				return fmt.Errorf("cleanup %T %s: %w", target, key, err)
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("cleanup waiting for %T %s: %w", target, key, ctx.Err())
			case <-time.After(interval):
			}
		}
	}
	return nil
}
