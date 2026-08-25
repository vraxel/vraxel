-- Host inventory writes. Reads live in host.sql's GetHostByID, because
-- the facts are part of one host's detail and never fetched on their own.

-- name: UpsertHostFacts :exec
-- One statement, so the scalars and the lists land together or not at
-- all. The agent stays silent once it has sent a given inventory, so a
-- half-written report would persist until the next reconnect -- a host
-- showing a new NIC list beside the old CPU model, with nothing to say
-- which half is current.
--
-- The data-modifying CTE runs whether or not anything references it;
-- that is the documented behaviour, and it is what makes this one
-- statement instead of a transaction the caller has to remember to open.
WITH scalars AS (
    UPDATE hosts SET
        virtualization       = @virtualization,
        cpu_model            = @cpu_model,
        cpu_sockets          = @cpu_sockets,
        cpu_cores_per_socket = @cpu_cores_per_socket,
        cpu_threads_per_core = @cpu_threads_per_core,
        kernel_version       = @kernel_version,
        os_id                = @os_id,
        os_version_id        = @os_version_id,
        system_vendor        = @system_vendor,
        product_name         = @product_name,
        bios_version         = @bios_version,
        bios_date            = @bios_date,
        board_name           = @board_name,
        board_serial         = @board_serial,
        chassis_type         = @chassis_type,
        serial_number        = @serial_number,
        asset_tag            = @asset_tag,
        timezone             = @timezone,
        default_gateway      = @default_gateway
        -- updated_at is deliberately left alone. It means "an operator
        -- changed this record"; a machine describing itself is not that,
        -- and bumping it would make every host look edited once an hour.
    WHERE id = @host_id
)
INSERT INTO host_facts (host_id, nics, filesystems, block_devices, reported_at)
VALUES (@host_id, @nics, @filesystems, @block_devices, now())
ON CONFLICT (host_id) DO UPDATE SET
    nics          = EXCLUDED.nics,
    filesystems   = EXCLUDED.filesystems,
    block_devices = EXCLUDED.block_devices,
    reported_at   = EXCLUDED.reported_at;
