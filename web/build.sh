set -e

# cleanup
rm -rf prod

# prep dir create
mkdir prod prod/dist prod/plugins

# copy over dist
cp -a dev/dist prod/.
rm -rf prod/dist/css/alt

# copy over html
cp dev/*html prod/.

# fontawesome
mkdir -p prod/plugins/fontawesome-free/css prod/plugins/fontawesome-free/webfonts
cp -a dev/plugins/fontawesome-free/css/*.min.css prod/plugins/fontawesome-free/css/.
cp -a dev/plugins/fontawesome-free/webfonts prod/plugins/fontawesome-free/.

# jquery
mkdir -p prod/plugins/jquery
cp -a dev/plugins/jquery/*.min.* prod/plugins/jquery/.

# bootstrap
mkdir -p prod/plugins/bootstrap/js
cp -a dev/plugins/bootstrap/js/*.min.* prod/plugins/bootstrap/js

# summary
du -hs *
