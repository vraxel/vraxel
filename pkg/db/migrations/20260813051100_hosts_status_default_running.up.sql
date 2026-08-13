-- One vocabulary for hosts.status.
--
-- The column defaulted to 'active' while its only writer (CreateAgentHost)
-- states 'running'. The default never fires today, so nothing is wrong at
-- runtime; it is wrong as a definition, and the next insert path that
-- omits status would quietly introduce a second word for one state.
ALTER TABLE hosts ALTER COLUMN status SET DEFAULT 'running';
