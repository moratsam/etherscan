from cockroachdb/cockroach

COPY txgraph/store/cdb/migrations /migrations
COPY depl/cdb-schema/bootstrap-db.sh .

ENTRYPOINT ["bash", "./bootstrap-db.sh"]
