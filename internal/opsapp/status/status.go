package status

import (
	"fmt"

	"kube-env/internal/opsapp/config"
	"kube-env/internal/opsapp/render"
	"kube-env/internal/opsapp/state"
)

const (
	SyncUnknown   = "Unknown"
	SyncInSync    = "InSync"
	SyncOutOfSync = "OutOfSync"
)

type EnvironmentStatus struct {
	Environment     string                  `json:"environment"`
	EnvName         string                  `json:"env_name"`
	Namespace       string                  `json:"namespace,omitempty"`
	TargetRevision  string                  `json:"target_revision,omitempty"`
	DesiredDigest   string                  `json:"desired_digest"`
	AppliedRevision string                  `json:"applied_revision,omitempty"`
	AppliedDigest   string                  `json:"applied_digest,omitempty"`
	SyncStatus      string                  `json:"sync_status"`
	Reason          string                  `json:"reason,omitempty"`
	Render          render.DigestResult     `json:"render"`
	AppliedState    *state.EnvironmentState `json:"applied_state,omitempty"`
}

func Compute(cfg config.Config, store state.Store, name string) (EnvironmentStatus, error) {
	env, ok := cfg.Environment(name)
	if !ok {
		return EnvironmentStatus{}, fmt.Errorf("environment %q not found", name)
	}
	desired, err := render.RenderDigestByName(cfg, name)
	if err != nil {
		return EnvironmentStatus{}, err
	}
	applied, found, err := store.Environment(env.Name)
	if err != nil {
		return EnvironmentStatus{}, err
	}
	result := EnvironmentStatus{
		Environment:    env.Name,
		EnvName:        env.EnvName,
		Namespace:      env.Namespace,
		TargetRevision: env.TargetRevision,
		DesiredDigest:  desired.Digest,
		SyncStatus:     SyncUnknown,
		Reason:         "no applied state recorded",
		Render:         desired,
	}
	if !found || applied.AppliedDigest == "" {
		return result, nil
	}
	result.AppliedRevision = applied.AppliedRevision
	result.AppliedDigest = applied.AppliedDigest
	result.AppliedState = &applied
	if applied.AppliedDigest == desired.Digest {
		result.SyncStatus = SyncInSync
		result.Reason = "desired digest matches applied digest"
	} else {
		result.SyncStatus = SyncOutOfSync
		result.Reason = "desired digest differs from applied digest"
	}
	return result, nil
}
