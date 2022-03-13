from cockroachdb/cockroach

COPY scorestore/cdb/migrations /migrations
COPY txgraph/store/cdb/migrations /migrations
COPY depl/cdb-schema/bootstrap-db.sh .

ENTRYPOINT ["bash", "./bootstrap-db.sh"]
