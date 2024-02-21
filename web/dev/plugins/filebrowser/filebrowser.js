/**@license
 *
 * jQuery File Browser - directory browser jQuery plugin version {{VER}}
 *
 * Copyright (c) 2016-2021 Jakub T. Jankiewicz <https://jcubic.pl/me>
 * Released under the MIT license
 *
 * Date: {{DATE}}
 */
/* global setTimeout jQuery File Directory */
(function($, undefined) {
    'use strict';
    function Uploader(browser, upload, error) {
        this.browser = browser;
        this.upload = upload;
        this.error = error;
    }

    Uploader.prototype.process = function process(event, path) {
        var defered = $.Deferred();
        var self = this;
        if (event.originalEvent) {
            event = event.originalEvent;
        }
        var items;
        if (event.dataTransfer.items) {
            items = [].slice.call(event.dataTransfer.items);
        }
        var files = (event.dataTransfer.files || event.target.files);
        if (files) {
            files = [].slice.call(files);
        }
        if (items && items.length) {
            if (items[0].webkitGetAsEntry) {
                var entries = [];
                items.forEach(function(item) {
                    var entry = item.webkitGetAsEntry();
                    if (entry) {
                        entries.push(entry);
                    }
                });
                (function upload() {
                    var entry = entries.shift();
                    if (entry) {
                        self.upload_tree(entry, path).then(upload);
                    } else {
                        defered.resolve();
                    }
                })();
            }
        } else if (files && files.length) {
            (function upload() {
                var file = files.shift();
                if (file) {
                    self.upload(file, path).then(upload);
                } else {
                    defered.resolve();
                }
            })();
        } else if (event.dataTransfer.getFilesAndDirectories) {
            event.dataTransfer.getFilesAndDirectories().then(function(items) {
                (function upload() {
                    var item = items.shift();
                    if (item) {
                        self.upload_tree(item, path).then(upload);
                    }  else {
                        defered.resolve();
                    }
                })();
            });
        }
        return defered.promise();
    };

    Uploader.prototype.upload_tree = function upload_tree(tree, path) {
        var defered = $.Deferred();
        var self = this;
        function process(entries, callback) {
            entries = entries.slice();
            (function recur() {
                var entry = entries.shift();
                if (entry) {
                    callback(entry).then(recur).fail(function() {
                        defered.reject();
                    });
                } else {
                    defered.resolve();
                }
            })();
        }
        function upload_files(entries) {
            process(entries, function(entry) {
                return self.upload_tree(entry, self.browser.join(path,tree.name));
            });
        }
        function upload_file(file) {
            self.upload(file, path).then(function() {
                defered.resolve();
            }).fail(function() {
                defered.reject();
            });
        }
        if (typeof Directory != 'undefined' && tree instanceof Directory) { // firefox
            tree.getFilesAndDirectories().then(function(entries) {
                upload_files(entries);
            });
        } else if (typeof File != 'undefined' && tree instanceof File) { // firefox
            upload_file(tree);
        } else if (tree.isFile) { // chrome
            tree.file(upload_file);
        } else if (tree.isDirectory) { // chrome
            var dirReader = tree.createReader();
            dirReader.readEntries(function(entries) {
                upload_files(entries);
            });
        }
        return defered.promise();
    };

    $.browse = {
        defaults: {
            dir: function() {
                return $.when({files:[], dirs: []});
            },
            root: '/',
            separator: '/',
            labels: true,
            change: $.noop,
            init: $.noop,
            item_class: $.noop,
            rename_delay: 300,
            dbclick_delay: 2000,
            open: $.noop,
            rename: $.noop,
            create: $.noop,
            remove: $.noop,
            copy: $.noop,
            exists: $.noop,
            upload: $.noop,
            name: 'default',
            error: $.noop,
            menu: function(type) {
                return {};
            },
            refresh_timer: 100
        },
        strings: {
            toolbar: {
                back: 'back',
                up: 'up',
                refresh: 'refresh'
            }
        },
        escape_regex: function(str) {
            if (typeof str == 'string') {
                var special = /([-\\\^$\[\]()+{}?*.|])/g;
                return str.replace(special, '\\$1');
            }
        }
    };
    var copy;
    var cut;
    var selected = {};
    var drag;
    function is(is_value) {
        return function(name) {
            return is_value == name;
        };
    }
    function all_parents_fun(fun, element) {
        var $element = $(element);
        return $element.parents().add('html,body').map(function() {
            return $(this)[fun]();
        }).get().reduce(function(sum, prop) {
            return sum + prop;
        });
    }
    function same_root(self, src, dest) {
        if (src === dest) {
            return true;
        }
        dest = self.join.apply(self, self.split(dest).slice(0, -1));
        return !!dest.match(new RegExp('^' + $.browse.escape_regex(src)));
    }
    $.fn.browse = function(options) {
        var settings = $.extend({}, $.browse.defaults, options);
        function mousemove(e) {
            if (selection) {
                var offset = $ul.offset();
                x2 = e.clientX - offset.left;
                y2 = e.clientY - offset.top;
                $selection.show();
                draw_selection();
                was_selecting = true;
                var $li = $content.find('li');
                if (!e.ctrlKey) {
                    $li.removeClass('selected');
                    selected[settings.name] = [];
                }
                var selection_rect = $selection[0].getBoundingClientRect();
                var $selected = $li.filter(function() {
                    var rect = this.getBoundingClientRect();
                    return rect.top + rect.height > selection_rect.top &&
                        rect.left + rect.width > selection_rect.left &&
                        rect.bottom - rect.height < selection_rect.bottom &&
                        rect.right - rect.width < selection_rect.right;
                });
                $selected.addClass('selected').each(function() {
                    selected[settings.name].push(self.join(path, $(this).text()));
                });
            }
        }
        function mousedown(e) {
            if (!$(e.target).closest('.browser-menu').length) {
                hide_menus();
            }
        }
        function mouseup(e) {
            selection = false;
            $selection.hide();
            self.removeClass('no-select');
        }
        function draw_selection(e) {
            var top = all_parents_fun('scrollTop', $content);
            var x3 = Math.max(Math.min(x1, x2), 0);
            var y3 = Math.max(Math.min(y1, y2), -top);
            var x4 = Math.max(x1, x2);
            var y4 = Math.max(y1, y2);
            var width = $content.prop('clientWidth');
            var height = $content.height() + $content.scrollTop() - 2;
            if (x4 > width) {
                x4 = width;
            }
            if (y4 > height) {
                y4 = height;
            }
            $selection.css({
                left: x3,
                top: y3 + top,
                width: x4 - x3,
                height: y4 - y3
            });
        }
        function keydown(e) {
            if (self.hasClass('selected') && !$(e.target).is('textarea')) {
                var current_item;
                var $active = $content.find('.active');
                if (e.ctrlKey) {
                    if (e.which == 67) { // CTRL+C
                        self.copy();
                    } else if (e.which == 88) { // CTRL+X
                        self.cut();
                    } else if (e.which == 86) { // CTRL+V
                        self.paste(cut);
                    }
                }
                if (e.which == 32) { // SPACE
                    var e = jQuery.Event("click");
                    e.ctrlKey = true;
                    e.target = $active[0];
                    $active.trigger(e);
                    return false;
                } else if (e.which == 8) { // BACKSPACE
                    self.back();
                } else {
                    if (e.which == 13 && $active.length) {
                        click_time = (new Date()).getTime();
                        $active.dblclick();
                    } else {
                        if (e.which >= 37 && e.which <= 40) {
                            if (!e.ctrlKey) {
                                $content.find('li').removeClass('selected');
                            }
                            if (!$active.length) {
                                $active = $content.find('li:eq(0)').addClass('active');
                            } else {
                                var $li = $content.find('li');
                                var browse_width = $content.prop('clientWidth');
                                var length = $li.length;
                                var width = $content.find('li:eq(0)').outerWidth(true);
                                var each_row = Math.floor(browse_width/width);
                                current_item = $active.index();
                                if (e.which == 37) { // LEFT
                                    current_item--;
                                } else if (e.which == 38) { // UP
                                    current_item = current_item-each_row;
                                } else if (e.which == 39) { // RIGHT
                                    current_item++;
                                } else if (e.which == 40) { // DOWN
                                    current_item = current_item+each_row;
                                }
                                if (current_item < 0) {
                                    current_item = 0;
                                } else if (current_item > length-1) {
                                    current_item = length-1;
                                }
                                $li.eq(current_item).addClass('active')
                                    .siblings().removeClass('active');
                            }
                        }
                    }
                }
            }
        }
        function click(e) {
            if (!$(e.target).closest('.' + cls).length) {
                $('.browser-widget').removeClass('selected');
            }
            var $target = $(e.target);
            var $menu_li = $target.closest('.browser-menu li');
            if ($menu_li.length) {
                if (context_menu_object) {
                    var $li = context_menu_object.target.closest('ul:not(.menu) li');
                    var menu = context_menu_object.menu;
                    $menu_li.parents('.ui-menu-item').addBack().each(function() {
                        menu = menu[$(this).find('> div').text()];
                    });
                    if (!$menu_li.find('> ul').length) {
                        hide_menus();
                    }
                    if ($.isFunction(menu)) {
                        setTimeout(function() {
                            menu.call(self, $li);
                        }, 0);
                    }
                    return false;
                }
            } else if (!$target.closest('.browser-menu').length ||
                $target.closest('.browser-menu li').length) {
                hide_menus();
            }
        }
        function refresh_same() {
            $('.'+cls).each(function() {
                var self = $(this).browse();
                if (self.path() == path && self.name() == settings.name) {
                    self.refresh();
                }
            });
        }
        function process_textarea(remove) {
            var $textarea = $(this);
            var $li = $textarea.parent();
            var new_name = $textarea.val();
            if ($li.hasClass('rename')) {
                $textarea.remove();
                $li.removeClass('rename');
                var old_name = $li.find('span').text();
                if (new_name != old_name) {
                    self._rename(self.join(path, old_name),
                                 self.join(path, new_name)).then(refresh_same);
                }
            } else if ($li.hasClass('new')) {
                if (remove) {
                    $li.remove();
                } else {
                    var type;
                    if ($li.hasClass('directory')) {
                        type = 'directory';
                    } else {
                        type = 'file';
                    }
                    self.create(type, self.join(path, new_name));
                }
            }
        }
        function hide_menus() {
            if ($.fn.menu) {
                $('body > .browser-menu').menu('destroy').remove();
            }
            context_menu_object = null;
        }
        function scroll_to_bottom() {
            var scrollHeight;
            if ($content.prop) {
                scrollHeight = $content.prop('scrollHeight');
            } else {
                scrollHeight = $content.attr('scrollHeight');
            }
            $content.scrollTop(scrollHeight);
        }
        function make_menu(context, submenu) {
            var $ul = $('<ul/>');
            if (!submenu) {
                $ul.addClass('browser-menu');
            }
            Object.keys(context).forEach(function(name) {
                var $li = $('<li class="' + class_name(name) + '"><div>' + name + '</div></li>')
                    .appendTo($ul);
                if ($.isPlainObject(context[name])) {
                    $li.append(make_menu(context[name], true));
                }
            });
            return $ul;
        }
        function class_name(string) {
            return string.toLowerCase().replace(/\s+([^\s])/g, function(_, letter) {
                return '-' + letter;
            });
        }
        function trigger_rename($li) {
            if (!$li.is('.new, .rename')) {
                var name = $li.find('span').text();
                $('<textarea>'+name+'</textarea>').appendTo($li)
                    .focus().select();
                $li.addClass('rename');
            } else {
                $li.find('textarea').focus().select();
            }
        }
        function is_file_drop(event) {
            if (event.originalEvent) {
                event = event.originalEvent;
            }
            if (event.dataTransfer.items && event.dataTransfer.items.length) {
                return !![].filter.call(event.dataTransfer.items, function(item) {
                    return item.kind == 'file';
                }).length;
            } else {
                return event.dataTransfer.files && event.dataTransfer.files.length;
            }
        }
        if (this.data('browse')) {
            return this.data('browse');
        } else if (this.length > 1) {
            return this.each(function() {
                $.fn.browse.call($(this), settings);
            });
        } else {
            var cls = 'browser-widget';
            selected[settings.name] = selected[settings.name] || [];
            var self = this;
            self.addClass(cls + ' hidden');
            var uploader = new Uploader(self, settings.upload, settings.error);
            var path;
            var paths = [];
            var current_content;
            var click_time;
            var textarea = false;
            var num_clicks = 0;
            var $toolbar = $('<div class="toolbar"/>').appendTo(self);
            var $adress_bar = $('<div class="adress-bar"></div>').appendTo($toolbar);
            $('<button>Forward</button>').addClass('forward').appendTo($adress_bar);
            var $tools = $('<ul></ul>').appendTo($toolbar);
            if (settings.labels) {
                $tools.addClass('labels');
            }
            var $adress = $('<input />').appendTo($adress_bar);
            var toolbar = $.browse.strings.toolbar;
            Object.keys(toolbar).forEach(function(name) {
                $('<li/>').text(toolbar[name]).addClass(name).appendTo($tools);
            });
            var $content = $('<ul/>').wrap('<div/>').parent().addClass('content')
                .appendTo(self);
            var $ul = $content.find('ul');
            var x1 = 0, y1 = 0, x2 = 0, y2 = 0;
            var $selection = $('<div/>').addClass('selection').hide().appendTo($content);
            var selection = false;
            var was_selecting = false;
            var context_menu_object;
            var context_menu = {
                li: {
                    'rename': trigger_rename,
                    'delete': function($li) {
                        $.when.apply($, $content.find('li.selected').map(function() {
                            var name = $(this).find('span').text();
                            return settings.remove(self.join(path, name));
                        })).then(refresh_same);
                    }
                },
                'content': {
                    'new': {
                        'directory': function($li) {
                            self.create('Directory');
                        },
                        'file': function($li) {
                            self.create('File');
                        }
                    }
                }
            };
            $toolbar.on('click.browse', 'li', function() {
                var $this = $(this);
                if (!$this.hasClass('disabled')) {
                    var name = $this.text();
                    self[name]();
                }
            }).on('click', '.forward', function() {
                settings.open(path);
            }).on('keydown.browse', 'input', function(e) {
                if (e.which == 13) {
                    var $this = $(this);
                    var path = $this.val();
                    self.show(path);
                    return false;
                }
            });
            $content.on('dblclick.browse', 'li', function(e) {
                var $li = $(this);
                var time = ((new Date()).getTime() - click_time);
                if (time < settings.rename_delay && time < settings.dbclick_delay) {
                    var name = $li.find('span').text();
                    var filename = self.join(path, name);
                    if ($li.hasClass('directory')) {
                        $li.removeClass('selected');
                        self.show(filename);
                    } else if ($li.hasClass('file')) {
                        settings.open(filename);
                    }
                }
            }).on('click.browse', 'ul:not(.menu) > li', function(e) {
                if (!selection) {
                    var $target = $(e.target);
                    var $this = $(this);
                    var name = $this.find('span').text();
                    var filename = self.join(path, name);
                    if ($target.is('span')) {
                        if (num_clicks++ % 2  === 0) {
                            click_time = (new Date()).getTime();
                        } else {
                            var time = ((new Date()).getTime() - click_time);
                            if (time > settings.rename_delay &&
                                time < settings.dbclick_delay) {
                                trigger_rename($this);
                                return false;
                            }
                        }
                    } else {
                        click_time = (new Date()).getTime();
                    }
                    if (!e.ctrlKey) {
                        $this.siblings().removeClass('selected');
                    }
                    if (!$target.is('textarea')) {
                        $content.find('.active').removeClass('active');
                        $this.toggleClass('selected').addClass('active');
                        if ($this.hasClass('selected')) {
                            if (!e.ctrlKey) {
                                selected[settings.name] = [];
                            }
                            selected[settings.name].push(filename);
                        } else if (e.ctrlKey) {
                            selected[settings.name] = selected[settings.name]
                                .filter(is(filename));
                        } else {
                            selected[settings.name] = [];
                        }
                    }
                }
            }).on('keydown', 'textarea', function(e) {
                if (e.which == 13 || e.which == 27) { // ENTER
                    process_textarea.call(this, e.which == 27);
                }
                if ([13, 27].indexOf(e.which) != -1) {
                    return false;
                }
            }).on('contextmenu', function(e) {
                if (settings.contextmenu && !e.ctrlKey) {
                    hide_menus();
                }
                return false;
            });
            self.on('click.browse', function(e) {
                $('.' + cls).removeClass('selected');
                self.addClass('selected');
                var $target = $(e.target);
                if (!was_selecting) {
                    if (!e.ctrlKey && !$target.is('.content li') &&
                        !$target.closest('.toolbar').length) {
                        $content.find('li').removeClass('selected');
                        selected[settings.name] = [];
                    }
                }
                if (!$target.is('textarea')) {
                    $content.find('li.rename,li.new')
                        .find('textarea').each(process_textarea);
                }
            });
            self.on('dragover.browse', '.content', function(event) {
                if (event.originalEvent) {
                    event = event.originalEvent;
                }
                event.dataTransfer.dropEffect = "move";
                return false;
            }).on('dragstart', '.content li', function(e) {
                e.originalEvent.dataTransfer.setData('text', 'anything');
                var $this = $(this);
                var name = $this.text();
                drag = {
                    name: name,
                    node: $this,
                    path: path,
                    context: self
                };
                drag.selection = $this.hasClass('selected');
            });
            $content.on('drop.browse', function(e) {
                var $target = $(e.target);
                var new_path;
                if ($target.is('.directory')) {
                    new_path = self.join(path, $target.text());
                } else {
                    new_path = path;
                }
                if (is_file_drop(e)) {
                    uploader.process(e, new_path).then(function() {
                        if (!$target.is('.directory')) {
                            refresh_same();
                        }
                    });
                } else {
                    if (self.name() !== drag.context.name()) {
                        var msg = "You can't drag across different filesystems";
                        settings.error(msg);
                    }
                    var promise;
                    if (drag.selection) {
                        promise = $.when.apply($, selected[settings.name].map(function(src) {
                            var dest = self.join(new_path, self.split(src).pop());
                            if (!same_root(self, src, dest)) {
                                return self._rename(src, dest);
                            }
                        }));
                    } else {
                        var src = self.join(drag.path, drag.name);
                        var dest = self.join(new_path, drag.name);
                        promise = self._rename(src, dest);
                    }
                    promise.then(function() {
                        drag.context.refresh();
                        refresh_same();
                    });
                }
                return false;
            }).on('mousedown.browse', function(e) {
                var $target = $(e.target);
                if (!$target.is('li') && !$target.is('span') && !$target.is('textarea')) {
                    selection = true;
                    was_selecting = false;
                    self.addClass('no-select');
                    var offset = $ul.offset();
                    x1 = e.clientX - offset.left;
                    y1 = e.clientY - offset.top;
                }
            });
            $(document).on('click', click)
                .on('keydown', keydown)
                .on('mousedown', mousedown)
                .on('mousemove', mousemove)
                .on('mouseup', mouseup);
            $.extend(self, {
                path: function() {
                    return path;
                },
                name: function() {
                    return settings.name;
                },
                current: function() {
                    return current;
                },
                back: function() {
                    if (paths.length > 1) {
                        paths.pop();
                        self.show(paths[paths.length-1], {push: false});
                    }
                    return self;
                },
                destroy: function() {
                    self.off('.browse');
                    $(document)
                        .off('click', click)
                        .off('keydown', keydown)
                        .off('mousedown', mousedown)
                        .off('mousemove', mousemove)
                        .off('mouseup', mouseup);
                    $adress_bar.remove();
                    $content.remove();
                },
                _rename: function(src, dest) {
                    var same = same_root(self, src, dest);
                    if (!same) {
                        return $.when(settings.rename(src, dest));
                    } else {
                        return $.when();
                    }
                },
                _create: function(type, path) {
                    return $.when(settings.create(type, path));
                },
                _exists: function(path) {
                    return $.when(settings.exists(path));
                },
                create: function(type, path) {
                    var _class = class_name(type);
                    if (path == undefined) {
                        var $li = $(['<li class="new ' + _class + '" draggable="true">',
                             '  <span></span>',
                             '  <textarea/>',
                             '</li>'].join('')).appendTo($ul);
                        scroll_to_bottom();
                        $li.find('textarea').val('New ' + type).focus().select();
                        return $.when();
                    }
                    return self._exists(path).then(function(exists) {
                        if (exists == true) {
                            $content.find('li.new').remove();
                            setTimeout(function() {
                                settings.error(type + ' already exists');
                            }, 10);
                        } else {
                            return self._create(type, path).then(refresh_same);
                        }
                    });
                },
                _copy: function(src, dest) {
                    if (!same_root(self, src, dest)) {
                        return settings.copy(src, dest);
                    } else {
                        return $.when();
                    }
                },
                copy: function() {
                    copy = {
                        path: path,
                        contents: selected[settings.name],
                        source: self
                    };
                    cut = false;
                },
                cut: function() {
                    self.copy();
                    cut = true;
                },
                paste: function(cut) {
                    function process(widget, fn) {
                        return $.when.apply($, copy.contents.map(function(src) {
                            var name = widget.split(src).pop();
                            var dest = widget.join(path, name);
                            if (!same_root(self, src, dest)) {
                                return widget[fn](src, dest);
                            } else {
                                return $.when();
                            }
                        }));
                    }
                    if (copy && copy.contents && copy.contents.length) {
                        if (self.name() !== copy.source.name()) {
                            settings.error("You can't paste across different filesystems");
                        } else {
                            var promise;
                            if (cut) {
                                promise = process(self, '_rename');
                            } else {
                                promise = process(self, '_copy');
                            }
                            promise.then(function() {
                                copy.source.refresh();
                                refresh_same();
                            });
                        }
                    }
                },
                up: function() {
                    var dirs = self.split(path);
                    dirs.pop();
                    self.show(self.join.apply(self, dirs));
                    return self;
                },
                refresh: function() {
                    $content.addClass('hidden');
                    var timer = $.Deferred();
                    var callback = $.Deferred();
                    if (settings.refresh_timer) {
                        setTimeout(timer.resolve.bind(timer), settings.refresh_timer);
                    } else {
                        timer.resolve();
                    }
                    self.show(path, {
                        force: true,
                        push: false,
                        callback: function() {
                            callback.resolve();
                        }
                    });
                    $.when(timer, callback).then(function() {
                        $content.removeClass('hidden');
                    });
                },
                show: function(new_path, options) {
                    function process(content) {
                        if (run) {
                            return;
                        }
                        run = true;
                        if (!content) {
                            settings.error('Invalid directory');
                            self.removeClass('hidden');
                        } else {
                            current_content = content;
                            $ul.empty();
                            current_content.dirs.forEach(function(dir) {
                                var cls = settings.item_class(new_path, dir);
                                var $li = $('<li class="directory"><span>' + dir + '</span></li>').
                                        appendTo($ul).attr('draggable', true);
                                if (cls) {
                                    $li.addClass(cls);
                                }
                            });
                            current_content.files.forEach(function(file) {
                                var $li = $('<li class="file"><span>' + file + '</span></li>').
                                        appendTo($ul).attr('draggable', true);
                                if (file.match('.')) {
                                    $li.addClass(file.split('.').pop());
                                }
                                var cls = settings.item_class(new_path, file);
                                if (cls) {
                                    $li.addClass(cls);
                                }
                            });
                            self.removeClass('hidden');
                            var re = new RegExp($.browse.escape_regex(settings.separator) + '$');
                            if (!new_path.match(re) && new_path != settings.root) {
                                new_path += settings.separator;
                            }
                            $adress.val(new_path);
                            settings.change.call(self);
                            options.callback();
                        }
                    }
                    var defaults = {callback: $.noop, push: true, force: false};
                    options = $.extend({}, defaults, options);
                    if (path != new_path || options.force) {
                        self.addClass('hidden');
                        if (options.push) {
                            paths.push(new_path);
                        }
                        $toolbar.find('.up').toggleClass('disabled', new_path == settings.root);
                        $toolbar.find('.back').toggleClass('disabled', paths.length == 1);
                        path = new_path;
                        // don't break old API. promise based and callback should both work
                        var result = settings.dir(path, process);
                        if (result && result.then) {
                            result.then(process).catch(function() {
                                process();
                            });
                        }
                        var run = false;
                    }
                    return self;
                },
                join: function() {
                    var paths = [].slice.call(arguments);
                    var path = paths.map(function(path) {
                        var re = new RegExp($.browse.escape_regex(settings.separator) + '$', '');
                        return path.replace(re, '');
                    }).filter(Boolean).join(settings.separator);// + settings.separator;
                    var re = new RegExp('^' + $.browse.escape_regex(settings.root));
                    return re.test(path) ? path : settings.root + path;
                },
                split: function(filename) {
                    var root = new RegExp('^' + $.browse.escape_regex(settings.root));
                    var separator = new RegExp($.browse.escape_regex(settings.separator) + '$');
                    filename = filename.replace(root, '').replace(separator, '');
                    if (filename) {
                        return filename.split(settings.separator).filter(Boolean);
                    } else {
                        return [];
                    }
                },
                walk: function(filename, fn) {
                    var path = this.split(filename);
                    var result;
                    while(path.length) {
                        result = fn(path.shift(), !path.length, result);
                    }
                    return result;
                }
            });
            setTimeout(function() {
                var path = settings.start_directory || settings.root;
                self.show(path, {
                    callback: settings.init.bind(self)
                });
            }, 0);
            self.data('browse', self);
            return self;
        }
    };
})(jQuery);
