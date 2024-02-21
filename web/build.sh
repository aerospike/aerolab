set -e

# cleanup
rm -rf prod

# prep dir create
mkdir prod prod/dist prod/plugins

# copy over dist
cp -a dev/dist prod/.
rm -rf prod/dist/css/alt

# copy over html and other bits
cp dev/*html prod/.
cp dev/*js prod/.
cp dev/*css prod/.
grep 'webuiVersion' ../src/version.go |awk -F'"' '{print $2}' > prod/version.cfg

# fontawesome
mkdir -p prod/plugins/fontawesome-free/css prod/plugins/fontawesome-free/js
cp -a dev/plugins/fontawesome-free/css/*.min.css prod/plugins/fontawesome-free/css/.
cp -a dev/plugins/fontawesome-free/js/*.min.js prod/plugins/fontawesome-free/js/.
cp -a dev/plugins/fontawesome-free/webfonts prod/plugins/fontawesome-free/.

# jquery
mkdir -p prod/plugins/jquery
cp -a dev/plugins/jquery/*.min.* prod/plugins/jquery/.

# bootstrap
mkdir -p prod/plugins/bootstrap/js
cp -a dev/plugins/bootstrap/js/*.min.* prod/plugins/bootstrap/js

# select2
mkdir -p prod/plugins/select2/css prod/plugins/select2/js prod/plugins/select2-bootstrap4-theme
cp -a dev/plugins/select2/css/select2.min.css prod/plugins/select2/css/select2.min.css
cp -a dev/plugins/select2/js/select2.full.min.js prod/plugins/select2/js/select2.full.min.js
cp -a dev/plugins/select2-bootstrap4-theme/select2-bootstrap4.min.css prod/plugins/select2-bootstrap4-theme/select2-bootstrap4.min.css

# toastr
mkdir -p prod/plugins/toastr
cp -a dev/plugins/toastr/*min* prod/plugins/toastr/.

# js-cookie
cp -a dev/plugins/cookie prod/plugins/.

# datatables
cp -a dev/plugins/datatables-full prod/plugins/.

# xtermjs
cp -a dev/plugins/xtermjs prod/plugins/.

# filebrowser + jquery-ui
mkdir -p prod/plugins/jquery-ui
cp -a dev/plugins/jquery-ui/jquery-ui.min.css prod/plugins/jquery-ui/.
cp -a dev/plugins/jquery-ui/jquery-ui.min.js prod/plugins/jquery-ui/.
cp -a dev/plugins/filebrowser prod/plugins/.

# summary
cd prod
tar -zcf ../../src/webui/www.tgz *
cd ..
