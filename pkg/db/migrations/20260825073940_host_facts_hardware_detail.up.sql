-- Eight more scalars on the host.facts report, same split as the columns
-- beside them: single-valued and comparable across a fleet, so columns.
--
-- Seven of the eight are fields NetBox and bk-cmdb model as data somebody
-- TYPES IN -- asset tag, board part number and serial, enclosure form
-- factor, distribution and release, default gateway. A machine can answer
-- every one of them about itself, and a value a machine answers is right
-- more often than one a person transcribed once at racking.
--
-- No indexes. os_id is the only one anybody would group a fleet by, and
-- there is no filter reading it yet; an index with no query behind it is
-- a write cost with nothing on the other side. It goes in with the
-- filter, the way virtualization's did.
ALTER TABLE hosts
    -- /etc/os-release ID and VERSION_ID, kept apart from the display
    -- string in os. "Everything on EL8" is a comparison on these two;
    -- against the joined string it is a substring match that breaks on
    -- the first distro with a digit in its name.
    ADD COLUMN os_id           varchar(32)  NOT NULL DEFAULT '',
    ADD COLUMN os_version_id   varchar(32)  NOT NULL DEFAULT '',
    -- Firmware build date as DMI spells it (MM/DD/YYYY). Stored as text,
    -- not a date: it is displayed and never compared, and a vendor who
    -- ships a malformed one should not fail the whole report's write.
    ADD COLUMN bios_date       varchar(32)  NOT NULL DEFAULT '',
    -- The motherboard, which is not the system. A board swap changes
    -- these and leaves serial_number (the chassis asset) alone.
    ADD COLUMN board_name      varchar(128) NOT NULL DEFAULT '',
    ADD COLUMN board_serial    varchar(128) NOT NULL DEFAULT '',
    -- SMBIOS enclosure class, already collapsed by the agent to one of
    -- six words. Empty on every guest, which reports "Other".
    ADD COLUMN chassis_type    varchar(16)  NOT NULL DEFAULT '',
    -- The tag an operator burned into SMBIOS at provisioning: NetBox's
    -- asset_tag and bk-cmdb's bk_asset_id, read instead of retyped.
    ADD COLUMN asset_tag       varchar(64)  NOT NULL DEFAULT '',
    -- IPv4 next hop for 0.0.0.0/0. varchar(45) to match the other address
    -- columns, though this one cannot hold a v6 address.
    ADD COLUMN default_gateway varchar(45)  NOT NULL DEFAULT '';
