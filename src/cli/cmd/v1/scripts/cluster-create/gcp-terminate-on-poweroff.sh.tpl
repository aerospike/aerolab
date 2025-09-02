cat <<'EOF' > /usr/local/bin/poweroff
export NAME=$(curl -X GET http://metadata.google.internal/computeMetadata/v1/instance/name -H 'Metadata-Flavor: Google')
export ZONE=$(curl -X GET http://metadata.google.internal/computeMetadata/v1/instance/zone -H 'Metadata-Flavor: Google')
gcloud --quiet compute instances delete $NAME --zone=$ZONE
systemctl poweroff
EOF
chmod 755 /usr/local/bin/poweroff
rm -f /sbin/poweroff
ln -s /usr/local/bin/poweroff /sbin/poweroff
