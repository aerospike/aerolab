set -e
cat <<'EOF' >> /root/.bashrc
export LS_COLORS='rs=0:di=01;34:ln=01;36:mh=00:pi=40;33:so=01;35:do=01;35:bd=40;33;01:cd=40;33;01:or=40;31;01:mi=00:su=37;41:sg=30;43:ca=30;41:tw=30;42:ow=34;42:st=37;44:ex=01;32:*.tar=01;31:*.tgz=01;31:*.arc=01;31:*.arj=01;31:*.taz=01;31:*.lha=01;31:*.lz4=01;31:*.lzh=01;31:*.lzma=01;31:*.tlz=01;31:*.txz=01;31:*.tzo=01;31:*.t7z=01;31:*.zip=01;31:*.z=01;31:*.dz=01;31:*.gz=01;31:*.lrz=01;31:*.lz=01;31:*.lzo=01;31:*.xz=01;31:*.zst=01;31:*.tzst=01;31:*.bz2=01;31:*.bz=01;31:*.tbz=01;31:*.tbz2=01;31:*.tz=01;31:*.deb=01;31:*.rpm=01;31:*.jar=01;31:*.war=01;31:*.ear=01;31:*.sar=01;31:*.rar=01;31:*.alz=01;31:*.ace=01;31:*.zoo=01;31:*.cpio=01;31:*.7z=01;31:*.rz=01;31:*.cab=01;31:*.wim=01;31:*.swm=01;31:*.dwm=01;31:*.esd=01;31:*.jpg=01;35:*.jpeg=01;35:*.mjpg=01;35:*.mjpeg=01;35:*.gif=01;35:*.bmp=01;35:*.pbm=01;35:*.pgm=01;35:*.ppm=01;35:*.tga=01;35:*.xbm=01;35:*.xpm=01;35:*.tif=01;35:*.tiff=01;35:*.png=01;35:*.svg=01;35:*.svgz=01;35:*.mng=01;35:*.pcx=01;35:*.mov=01;35:*.mpg=01;35:*.mpeg=01;35:*.m2v=01;35:*.mkv=01;35:*.webm=01;35:*.webp=01;35:*.ogm=01;35:*.mp4=01;35:*.m4v=01;35:*.mp4v=01;35:*.vob=01;35:*.qt=01;35:*.nuv=01;35:*.wmv=01;35:*.asf=01;35:*.rm=01;35:*.rmvb=01;35:*.flc=01;35:*.avi=01;35:*.fli=01;35:*.flv=01;35:*.gl=01;35:*.dl=01;35:*.xcf=01;35:*.xwd=01;35:*.yuv=01;35:*.cgm=01;35:*.emf=01;35:*.ogv=01;35:*.ogx=01;35:*.aac=00;36:*.au=00;36:*.flac=00;36:*.m4a=00;36:*.mid=00;36:*.midi=00;36:*.mka=00;36:*.mp3=00;36:*.mpc=00;36:*.ogg=00;36:*.ra=00;36:*.wav=00;36:*.oga=00;36:*.opus=00;36:*.spx=00;36:*.xspf=00;36:'
EOF
override=%s
mkdir -p /opt/agi/aerospike/data
mkdir -p /opt/agi/aerospike/smd
uuidgen -r > /opt/agi/uuid
[ "%t" = "true" ] && touch /opt/agi/nodim || echo "DIM"
cat <<'EOF' > /opt/agi/owner
%s
EOF
set +e
which apt
ISAPT=$?
set -e
if [ $ISAPT -eq 0 ]
then
    ANS=1
    TOUT=0
    while [ $ANS -ne 0 ]
    do
        apt update && apt -y install wget adduser libfontconfig1 musl ssl-cert && wget -q https://dl.grafana.com/oss/release/grafana_%s_%s.deb && dpkg -i grafana_%s_%s.deb
        ANS=$?
        if [ $ANS -ne 0 ]
        then
            TOUT=$((TOUT + 1))
            [ $TOUT -gt 900 ] && exit 1
            sleep 1
        fi
    done
else
    yum install -y wget mod_ssl
    mkdir -p /etc/ssl/certs /etc/ssl/private
    openssl req -new -x509 -nodes -out /etc/ssl/certs/ssl-cert-snakeoil.pem -keyout /etc/ssl/private/ssl-cert-snakeoil.key -days 3650 -subj '/CN=www.example.com'
    yum install -y https://dl.grafana.com/oss/release/grafana-%s-1.%s.rpm
fi
[ ! -f /opt/agi/proxy.cert ] && cp /etc/ssl/certs/ssl-cert-snakeoil.pem /opt/agi/proxy.cert
[ ! -f /opt/agi/proxy.key ] && cp /etc/ssl/private/ssl-cert-snakeoil.key /opt/agi/proxy.key
chmod 755 /usr/local/bin/aerolab
aerolab config backend -t none
%s
[ ! -f /opt/agi/aerospike/features.conf ] && cp /etc/aerospike/features.conf /opt/agi/aerospike/features.conf
cat <<'EOF' > /etc/aerospike/aerospike.conf
service {
    proto-fd-max 15000
    work-directory /opt/agi/aerospike
    cluster-name agi
    feature-key-file /opt/agi/aerospike/features.conf
}
logging {
    file /var/log/agi-aerospike.log {
        context any info
    }
}
network {
    service {
        address any
        port 3000
    }
    heartbeat {
        interval 150
        mode mesh
        port 3002
        timeout 10
    }
    fabric {
        port 3001
    }
    info {
        port 3003
    }
}
namespace agi {
    default-ttl 0
    %s
    replication-factor 2
    storage-engine %s {
        file /opt/agi/aerospike/data/agi.dat
        filesize %dG
        %s
        %s
        %s
        %s
    }
}
EOF

echo "%s" > /opt/agi/label
echo "%s" > /opt/agi/name

if [ $override -eq 1 -o ! -f /etc/systemd/system/agi-plugin.service ]
then
cat <<'EOF' > /etc/systemd/system/agi-plugin.service
[Unit]
Description=AGI Plugin
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=on-failure
User=root
RestartSec=10
WorkingDirectory=/opt/agi
ExecStart=/usr/local/bin/aerolab agi exec plugin -y /opt/agi/plugin.yaml

[Install]
WantedBy=multi-user.target
EOF
fi

if [ $override -eq 1 -o ! -f /etc/systemd/system/agi-grafanafix.service ]
then
cat <<'EOF' > /etc/systemd/system/agi-grafanafix.service
[Unit]
Description=AGI Grafana Helper
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=on-failure
User=root
RestartSec=10
WorkingDirectory=/opt/agi
ExecStart=/usr/local/bin/aerolab agi exec grafanafix -y /opt/agi/grafanafix.yaml

[Install]
WantedBy=multi-user.target
EOF
fi

if [ $override -eq 1 -o ! -f /etc/systemd/system/agi-ingest.service ]
then
cat <<'EOF' > /etc/systemd/system/agi-ingest.service
[Unit]
Description=AGI Ingest
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=no
User=root
RestartSec=10
WorkingDirectory=/opt/agi
ExecStart=/usr/local/bin/aerolab agi exec ingest -y /opt/agi/ingest.yaml --agi-name %s

[Install]
WantedBy=multi-user.target
EOF
fi

if [ $override -eq 1 -o ! -f /etc/systemd/system/agi-proxy.service ]
then
cat <<'EOF' > /etc/systemd/system/agi-proxy.service
[Unit]
Description=AGI Proxy
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=on-failure
User=root
RestartSec=10
WorkingDirectory=/opt/agi
ExecStart=/usr/local/bin/aerolab agi exec proxy -c "%s" --agi-name %s -L "%s" -a token -l %d %s -C %s -K %s -m %s -M %s

[Install]
WantedBy=multi-user.target
EOF
fi

if [ $override -eq 1 -o ! -f /opt/agi/grafanafix.yaml ]
then
cat <<'EOF' > /opt/agi/grafanafix.yaml
dashboards:
  fromDir: ""
  loadEmbedded: true
grafanaURL: "http://127.0.0.1:8850"
annotationFile: "/opt/agi/annotations.json"
labelFiles:
  - "/opt/agi/label"
  - "/opt/agi/name"
EOF
fi

if [ $override -eq 1 -o ! -f /opt/agi/plugin.yaml ]
then
cat <<'EOF' > /opt/agi/plugin.yaml
maxDataPointsReceived: %d
logLevel: %d
addNoneToLabels:
  - Histogram
  - HistogramDev
  - HistogramUs
  - HistogramSize
  - HistogramCount
%s
EOF
fi

if [ $override -eq 1 -o ! -f /opt/agi/notifier.yaml ]
then
cat <<'EOF' > /opt/agi/notifier.yaml
%s
EOF
fi

if [ $override -eq 1 -o ! -f /opt/agi/ingest.yaml ]
then
    mv /tmp/ingest.yaml /opt/agi/ingest.yaml
fi

if [ $override -eq 1 -o ! -f /opt/agi/deployment.json.gz ]
then
    mv /tmp/deployment.json.gz /opt/agi/deployment.json.gz
fi
rm -f /tmp/ingest.yaml /tmp/deployment.json.gz

chmod 755 /etc/systemd/system/agi-plugin.service /etc/systemd/system/agi-proxy.service /etc/systemd/system/agi-grafanafix.service /etc/systemd/system/agi-ingest.service
systemctl enable agi-plugin && systemctl enable agi-proxy && systemctl enable agi-grafanafix && systemctl enable agi-ingest && systemctl daemon-reload
systemctl start agi-plugin ; systemctl start agi-proxy ; systemctl start agi-grafanafix ; systemctl start agi-ingest
set +e
rm -rf /root/agiinstaller.sh

cat <<'EOF'> /usr/local/bin/erro
grep -i 'error|warn|timeout' "$@"
EOF
chmod 755 /usr/local/bin/erro
date > /opt/agi-installed && exit 0 || exit 0
