-- +goose Up

-- Unlock output-phase transformers: they run on the LLM response before
-- output guards, mirroring the input pipeline.
ALTER TABLE transformers DROP CONSTRAINT transformers_phase_check;
ALTER TABLE transformers ADD CONSTRAINT transformers_phase_check CHECK (phase IN ('input', 'output'));

-- +goose Down

ALTER TABLE transformers DROP CONSTRAINT transformers_phase_check;
ALTER TABLE transformers ADD CONSTRAINT transformers_phase_check CHECK (phase = 'input');
