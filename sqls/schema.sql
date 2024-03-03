CREATE TYPE process_status AS ENUM ('RUNNING', 'STOPPED', 'CRASHED', 'STARTING', 'STOPPING', 'STOPPED_WILL_RESTART', 'CRASHED_WILL_RESTART', 'UNKNOWN');
CREATE TYPE process_event_type AS ENUM ('UNKNOWN', 'START', 'STOP', 'CRASH', 'FULL_STOP', 'FULL_CRASH', 'MANUALLY_STOPPED', 'RESTART');

CREATE TABLE IF NOT EXISTS process_group
(
    id                    SERIAL PRIMARY KEY,
    name                  VARCHAR(255) NOT NULL,
    color                 VARCHAR(9) DEFAULT NULL,

    scripts_configuration JSONB
);


CREATE TABLE IF NOT EXISTS process
(
    id                SERIAL PRIMARY KEY,
    name              VARCHAR(255)   NOT NULL,
    process_group_id  INTEGER        REFERENCES process_group (id) ON DELETE SET NULL DEFAULT NULL,
    color             VARCHAR(9)                                                      DEFAULT NULL,
    enabled           BOOLEAN        NOT NULL                                         DEFAULT TRUE,

    executable_path   VARCHAR(512)   NOT NULL,
    arguments         VARCHAR(4096)  NOT NULL,
    working_directory VARCHAR(512)   NOT NULL,
    environment       JSONB          NOT NULL,

    status            process_status NOT NULL                                         DEFAULT 'UNKNOWN',

    configuration     JSONB
);



CREATE TABLE IF NOT EXISTS process_event
(
    id              SERIAL PRIMARY KEY,
    process_id      INTEGER REFERENCES process (id) ON DELETE CASCADE,
    event           process_event_type NOT NULL DEFAULT 'UNKNOWN',
    created_at      TIMESTAMP          NOT NULL DEFAULT CURRENT_TIMESTAMP,
    additional_info JSONB
);

CREATE TABLE IF NOT EXISTS process_stats
(
    id                   SERIAL PRIMARY KEY,
    process_id           INTEGER REFERENCES process (id) ON DELETE CASCADE,
    created_at           TIMESTAMP        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cpu_usage            BIGINT           NOT NULL,
    cpu_usage_percentage FLOAT         NOT NULL,
    memory_usage         BIGINT           NOT NULL
);


CREATE TABLE IF NOT EXISTS logs
(
    id         SERIAL PRIMARY KEY,
    process_id INTEGER REFERENCES process (id) ON DELETE CASCADE,
    start_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    end_time   TIMESTAMP DEFAULT NULL,
    path       VARCHAR(512) NOT NULL
);
