-- The host's whole disk footprint: bytes used and bytes total, summed
-- over the real filesystems (the same tmpfs/ramfs exclusion disk_used_pct
-- already applies). This is the inventory answer the host list needs --
-- "how much disk does this machine have and how much of it is gone" --
-- which disk_used_pct cannot give: that one is the FULLEST single
-- filesystem, deliberately, so an alert fires when any one mount fills
-- rather than when the average does. The two answer different questions
-- and both are kept.
--
-- NULL rather than 0 default: an agent too old to report them has not
-- said the host has no disk, and 0 would render as an empty machine.
ALTER TABLE host_metrics_latest
    ADD COLUMN disk_used_bytes  bigint,
    ADD COLUMN disk_total_bytes bigint;
