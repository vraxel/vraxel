-- The supervisor's view of what this machine runs, beside the process
-- table's view of it.
--
-- On host_processes and not host_facts, because the two answer the same
-- question from opposite sides and must be read together: the process
-- list shows what is alive, the unit list shows what was SUPPOSED to be,
-- and the finding -- something enabled that is not running -- exists only
-- in the difference. They also need the same freshness. Facts is sampled
-- hourly; "the service died" an hour ago is not an answer anybody wants,
-- and this report is already on the five-minute loop and already served
-- by a verb that reads the host live when its agent is reachable.
--
-- [{name,enabled,active,sub,description}]. No timestamps and no pids:
-- this report is sent only when its content differs from the last one,
-- and a field that varies per sample would defeat that.
ALTER TABLE host_processes
    ADD COLUMN units jsonb NOT NULL DEFAULT '[]';
