
    CREATE TABLE request_log (
        day          TEXT    NOT NULL,
        route        TEXT    NOT NULL,
        status_class TEXT    NOT NULL,
        count        INTEGER NOT NULL,
        PRIMARY KEY (day, route, status_class)
    );
    