-- +goose Up

-- Pass criteria become per-phase: input and output guard sets rarely have
-- the same guard count, so a single n_of_m threshold can be unsatisfiable
-- for one phase. Existing values are copied to both phases, preserving
-- behavior.
ALTER TABLE sources RENAME COLUMN pass_criteria TO input_pass_criteria;
ALTER TABLE sources RENAME COLUMN pass_threshold TO input_pass_threshold;
ALTER TABLE sources ADD COLUMN output_pass_criteria TEXT NOT NULL DEFAULT 'all';
ALTER TABLE sources ADD COLUMN output_pass_threshold INT;

UPDATE sources
SET output_pass_criteria = input_pass_criteria,
    output_pass_threshold = input_pass_threshold;

-- +goose Down

ALTER TABLE sources DROP COLUMN output_pass_threshold;
ALTER TABLE sources DROP COLUMN output_pass_criteria;
ALTER TABLE sources RENAME COLUMN input_pass_threshold TO pass_threshold;
ALTER TABLE sources RENAME COLUMN input_pass_criteria TO pass_criteria;
