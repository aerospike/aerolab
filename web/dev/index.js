{{define "mainjs"}}
function menuSetActive(id) {
    // get current item
    current = $("#"+id);
    // if current item is an expanding menu item, do exit
    classList = current.parent().attr("class").split(/\s+/);
    if (classList.includes("menu-closed")) {
        return;
    }
    // remove all active classes
    menu = $("#mainMenu")
    menu.find("a").each(function () {
        $(this).removeClass("active");
        $(this).removeClass("bg-blue");
        $(this).removeClass("bg-white");
    });
    // add active class to current item
    current.addClass("active");
    current.addClass("bg-blue");
    // recursively add active class to parent menu items
    menuid = menu.attr("id");
    while (current.attr("id") != menuid) {
        current = current.parent();
        if ($(current).attr("class").split(/\s+/).includes("menu-closed")) {
            $(current).children("a").each(function () {
                $(this).addClass("active");
                $(this).addClass("bg-white");
            });
        }
    }
    // TODO: actually do something in the main section
}
{{end}}
