-- name: CreateProcess :one
INSERT INTO process (name, process_group_id, color, executable_path, arguments, working_directory, environment,
                     configuration, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: GetProcess :one
SELECT *
FROM process
WHERE id = $1;

-- name: GetProcessByName :many
SELECT *
FROM process
WHERE name = $1;

-- name: GetProcesses :many
SELECT *
FROM process
ORDER BY id ASC;

-- name: UpdateProcess :one
UPDATE process
SET name=$2,
    process_group_id=$3,
    color=$4,
    executable_path=$5,
    arguments=$6,
    working_directory=$7,
    environment=$8,
    configuration=$9,
    enabled=$10
WHERE id = $1 RETURNING *;

-- name: SetProcessStatus :exec
UPDATE process
SET status=$2
WHERE id = $1;

-- name: SetProcessEnabled :exec
UPDATE process
SET enabled=$2
WHERE id = $1;

-- name: SetProcessConfiguration :exec
UPDATE process
SET configuration=$2
WHERE id = $1;

-- name: DeleteProcess :exec
DELETE
FROM process
WHERE id = $1;

-- name: CreateProcessGroup :one
INSERT INTO process_group (name, color, scripts_configuration)
VALUES ($1, $2, $3) RETURNING *;

-- name: GetProcessGroup :one
SELECT *
FROM process_group
WHERE id = $1
ORDER BY id ASC;

-- name: GetProcessGroupByName :many
SELECT *
FROM process_group
WHERE name = $1
ORDER BY id ASC;

-- name: GetProcessGroups :many
SELECT *
FROM process_group
ORDER BY id ASC;

-- name: UpdateProcessGroup :one
UPDATE process_group
SET name=$2,
    color=$3,
    scripts_configuration=$4
WHERE id = $1 RETURNING *;

-- name: DeleteProcessGroup :exec
DELETE
FROM process_group
WHERE id = $1;

-- name: GroupExistsByName :one
SELECT EXISTS(SELECT 1
              FROM process_group
              WHERE name = $1) AS exists;

-- name: GetGroupsByName :many
SELECT *
FROM process_group
WHERE name = $1;

-- name: InsertProcessEvent :one
INSERT INTO process_event (process_id, event, additional_info)
VALUES ($1, $2, $3) RETURNING *;

-- name: GetProcessEvents :many
SELECT *
FROM process_event
WHERE process_id = $1
ORDER BY id ASC;

-- name: GetProcessEventsAfter :many
SELECT *
FROM process_event
WHERE process_id = $1
  AND created_at >= $2
ORDER BY id ASC;

-- name: GetProcessEventsFromTo :many
SELECT *
FROM process_event
WHERE process_id = $1
  AND created_at >= $2
  AND created_at <= $3
    ORDER BY id DESC
    LIMIT $4;



-- name: GetAllProcessEvents :many
SELECT *
FROM process_event
ORDER BY id ASC;

-- name: InsertProcessStats :one
INSERT INTO process_stats (process_id, cpu_usage, cpu_usage_percentage, memory_usage)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetProcessStats :many
SELECT *
FROM process_stats
WHERE process_id = $1
ORDER BY id ASC;


-- name: GetProcessStatsFromTo :many
SELECT *
FROM process_stats
WHERE process_id = $1
  AND created_at >= $2
  AND created_at <= $3
ORDER BY id;


-- name: GetProcessesByGroup :many
SELECT *
FROM process
WHERE process_group_id = $1
ORDER BY id ASC;


-- name: LastProcessLogFile :one
SELECT *
FROM logs
WHERE process_id = $1
ORDER BY id DESC LIMIT 1;

-- name: NewProcessLogFile :one
INSERT INTO logs (process_id, path)
VALUES ($1, $2) RETURNING *;

-- name: GetLogFiles :many
SELECT *
FROM logs
WHERE process_id = $1
ORDER BY id ASC;

-- name: GetAllLogFiles :many
SELECT *
FROM logs
WHERE process_id=$1
ORDER BY id;

-- name: GetLogFilesFromTo :many
SELECT *
FROM logs
WHERE process_id = $1
  AND start_time >= $2
  AND (end_time <= $3 OR end_time IS NULL)
ORDER BY id;

-- name: SetLogEndTime :exec
UPDATE logs
SET end_time=$2
WHERE id = $1;
