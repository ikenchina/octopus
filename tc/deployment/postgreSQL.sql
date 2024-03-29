
CREATE SCHEMA dtx;

CREATE TYPE dtx.txn_state AS ENUM (
    'prepared',
    'committing',
    'precommitted',
    'committed',
    'rolling',
    'preaborted',
    'aborted'
);

CREATE TYPE dtx.txn_call_type AS ENUM (
    'sync',
    'async'
);

CREATE TYPE dtx.txn_type AS ENUM (
    'tcc',
    'saga'
);


CREATE TABLE IF NOT EXISTS dtx.global_txn(
    id BIGSERIAL PRIMARY key,
    gtid character varying(32) NOT NULL ,
    business character varying(32),
    state dtx.txn_state DEFAULT 'prepared'::dtx.txn_state NOT NULL,
    txn_type dtx.txn_type NOT NULL,
    updated_time TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
	created_time TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
    expire_time TIMESTAMP WITH TIME ZONE NOT NULL,
    lease_expire_time TIMESTAMP WITH TIME ZONE,
    lessee character varying(32),
    call_type dtx.txn_call_type DEFAULT 'sync'::dtx.txn_call_type NOT NULL,
    notify_action character varying(1024),
    notify_timeout BIGINT,
    notify_retry BIGINT,
    notify_count INT,
    parallel_execution BOOLEAN DEFAULT false NOT NULL
);
CREATE UNIQUE INDEX global_txn_gtid_idx ON dtx.global_txn(gtid);
CREATE INDEX global_txn_leaseexpire_state_running ON dtx.global_txn(lease_expire_time) WHERE state NOT IN ('committed', 'aborted', 'prepared');
CREATE INDEX global_txn_expire_state_prepared ON dtx.global_txn(expire_time) WHERE state IN ('prepared');
CREATE INDEX global_txn_expire_state_end ON dtx.global_txn(expire_time) WHERE state IN ('committed', 'aborted');


CREATE TYPE dtx.branch_type AS ENUM (
    'commit',
    'compensation',
    'confirm',
    'cancel'
);


CREATE TABLE IF NOT EXISTS dtx.branch_action(
    id BIGSERIAL PRIMARY key,
    gtid character varying(32) NOT NULL,
    bid INT NOT NULL,
    branch_type dtx.branch_type NOT NULL,
    action character varying(1024) NOT NULL,
    payload bytea,
    timeout BIGINT NOT NULL,
    response bytea,
    retry TEXT,
    try_count INT DEFAULT 0,
    state dtx.txn_state DEFAULT 'prepared'::dtx.txn_state NOT NULL,
    updated_time TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
	created_time TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT gtid_bid_type_branch_action UNIQUE (gtid, bid, branch_type)
);
CREATE INDEX branch_action_gtid_idx ON dtx.branch_action(gtid);
