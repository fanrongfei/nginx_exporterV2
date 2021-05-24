# nginx_exporterV2
nginx log_format must match this :
log_format  main  '$remote_addr - $remote_user [$time_local] "$request" '
                  '$status $upstream_response_time "$http_referer" '
                  '"$http_user_agent" "$http_x_forwarded_for" "$body_bytes_sent"';
