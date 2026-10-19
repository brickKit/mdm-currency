-- Currencies (ISO 4217, the code is the key: protocol 14's standard code tables), rate types and daily exchange rates.
-- Every record has a version (protocol 02, S1) and is published as a full-state event in the same transaction.
-- search is the normalized search column of the list keyword (protocol 14): code and names; rows written by a
-- migration have none yet and get it from the backfill job (search_version below the SDK's).
CREATE TABLE currency (
    code           text PRIMARY KEY CHECK (code ~ '^[A-Z]{3}$'),
    numeric_code   text NOT NULL CHECK (numeric_code ~ '^[0-9]{3}$'),
    minor_unit     smallint NOT NULL CHECK (minor_unit BETWEEN 0 AND 4),
    rounding       text NOT NULL DEFAULT 'half_away_from_zero' CHECK (rounding IN ('half_away_from_zero', 'half_even')),
    cash_unit      numeric(19,4) CHECK (cash_unit > 0),
    active         boolean NOT NULL DEFAULT true,
    name           text NOT NULL CHECK (name <> ''),
    names          jsonb NOT NULL DEFAULT '{}'::jsonb,
    search         text,
    search_version integer,
    version        bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX currency_search_due ON currency (code) WHERE search_version IS NULL;

CREATE TABLE rate_type (
    code       text PRIMARY KEY CHECK (code ~ '^[a-z][a-z0-9_]{0,31}$'),
    active     boolean NOT NULL DEFAULT true,
    name       text NOT NULL CHECK (name <> ''),
    names      jsonb NOT NULL DEFAULT '{}'::jsonb,
    version    bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- one rate per type, date and pair (the boundary card's invariant); a correction is a new version of the same row
CREATE TABLE exchange_rate (
    id            uuid PRIMARY KEY,
    rate_type     text NOT NULL REFERENCES rate_type (code),
    rate_date     date NOT NULL,
    from_currency text NOT NULL REFERENCES currency (code),
    to_currency   text NOT NULL REFERENCES currency (code) CHECK (to_currency <> from_currency),
    rate          numeric(19,10) NOT NULL CHECK (rate > 0),
    source        text NOT NULL CHECK (source <> ''),
    version       bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (rate_type, from_currency, to_currency, rate_date)
);
-- the effective rate: the latest date not after the requested one, by type and pair (the UNIQUE index serves it)
-- the list, latest first
CREATE INDEX exchange_rate_by_date ON exchange_rate (rate_date DESC, id DESC);
