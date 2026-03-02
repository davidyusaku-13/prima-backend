CREATE TABLE IF NOT EXISTS admin_user_actions (
    id BIGSERIAL PRIMARY KEY,
    actor_clerk_id TEXT NOT NULL REFERENCES users(clerk_id),
    target_clerk_id TEXT NOT NULL REFERENCES users(clerk_id),
    action_type TEXT NOT NULL,
    reason TEXT NOT NULL,
    before_state JSONB NOT NULL DEFAULT '{}'::jsonb,
    after_state JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS admin_user_actions_target_created_idx
    ON admin_user_actions(target_clerk_id, created_at DESC);

CREATE INDEX IF NOT EXISTS admin_user_actions_actor_created_idx
    ON admin_user_actions(actor_clerk_id, created_at DESC);
