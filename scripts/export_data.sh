# https://github.com/elasticsearch-dump/elasticsearch-dump

mkdir -p /tmp/demo-data


mkdir -p /tmp/demo-data/es-demo
NEW_ELASTICSEARCH_ADDRESS="https://${ELASTICSEARCH_USERNAME}:${ELASTICSEARCH_PASSWORD}@${ELASTICSEARCH_ADDRESS#https://}"

curl -X GET "$ELASTICSEARCH_ADDRESS/_cat/indices?format=json" -u "$ELASTICSEARCH_USERNAME:$ELASTICSEARCH_PASSWORD" --insecure | jq -r '.[].index' | while read -r index; do
  if [ "$(echo "$index" | cut -c 1)" != "." ] && [ "${index#security-auditlog-}" = "$index" ]; then
    NODE_TLS_REJECT_UNAUTHORIZED=0 elasticdump \
      --input="$NEW_ELASTICSEARCH_ADDRESS/$index" \
      --output="/tmp/demo-data/es-demo/$index.settings.json" \
      --type=settings
    NODE_TLS_REJECT_UNAUTHORIZED=0 elasticdump \
      --input="$NEW_ELASTICSEARCH_ADDRESS/$index" \
      --output="/tmp/demo-data/es-demo/$index.mapping.json" \
      --type=mapping
    NODE_TLS_REJECT_UNAUTHORIZED=0 elasticdump \
      --input="$NEW_ELASTICSEARCH_ADDRESS/$index" \
      --output="/tmp/demo-data/es-demo/$index.json" \
      --type=data
  fi
done

mkdir -p /tmp/demo-data/postgres
pg_dump --dbname="postgresql://$OCT_POSTGRESQL_USERNAME:$OCT_POSTGRESQL_PASSWORD@$OCT_POSTGRESQL_HOST:$POSTGRESQL_PORT/pennywise" > /tmp/demo-data/postgres/pennywise.sql
pg_dump --dbname="postgresql://$OCT_POSTGRESQL_USERNAME:$OCT_POSTGRESQL_PASSWORD@$OCT_POSTGRESQL_HOST:$POSTGRESQL_PORT/workspace" > /tmp/demo-data/postgres/workspace.sql
pg_dump --dbname="postgresql://$OCT_POSTGRESQL_USERNAME:$OCT_POSTGRESQL_PASSWORD@$OCT_POSTGRESQL_HOST:$POSTGRESQL_PORT/auth" --exclude-table api_keys --exclude-table users --exclude-table configurations > /tmp/demo-data/postgres/auth.sql
pg_dump --dbname="postgresql://$POSTGRESQL_USERNAME:$POSTGRESQL_PASSWORD@$POSTGRESQL_HOST:$POSTGRESQL_PORT/migrator" > /tmp/demo-data/postgres/migrator.sql
pg_dump --dbname="postgresql://$POSTGRESQL_USERNAME:$POSTGRESQL_PASSWORD@$POSTGRESQL_HOST:$POSTGRESQL_PORT/describe" > /tmp/demo-data/postgres/describe.sql
pg_dump --dbname="postgresql://$POSTGRESQL_USERNAME:$POSTGRESQL_PASSWORD@$POSTGRESQL_HOST:$POSTGRESQL_PORT/onboard" > /tmp/demo-data/postgres/onboard.sql
pg_dump --dbname="postgresql://$POSTGRESQL_USERNAME:$POSTGRESQL_PASSWORD@$POSTGRESQL_HOST:$POSTGRESQL_PORT/inventory" > /tmp/demo-data/postgres/inventory.sql
pg_dump --dbname="postgresql://$POSTGRESQL_USERNAME:$POSTGRESQL_PASSWORD@$POSTGRESQL_HOST:$POSTGRESQL_PORT/compliance" > /tmp/demo-data/postgres/compliance.sql
pg_dump --dbname="postgresql://$POSTGRESQL_USERNAME:$POSTGRESQL_PASSWORD@$POSTGRESQL_HOST:$POSTGRESQL_PORT/metadata" > /tmp/demo-data/postgres/metadata.sql

cd /tmp

tar -cO demo-data | openssl enc -aes-256-cbc -md md5 -pass pass:"$OPENSSL_PASSWORD" -base64 > demo_data.tar.gz.enc

aws s3 cp /tmp/demo_data.tar.gz.enc "s3://opengovernance/demo-data/v1.0/demo_data.tar.gz.enc" --endpoint-url=https://nyc3.digitaloceanspaces.com