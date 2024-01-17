{{define "mainjs"}}
function menuSetActive(id) {
    // get current item
    let current = $(id);
    // if current item is an expanding menu item, we are not setting anything; exit
    if (current.parent().attr("class").split(/\s+/).includes("menu")) {
        return;
    }
    // fade in loading spinner
    $("#loadingSpinner").fadeIn();
    // reset: remove all active classes
    let menu = $("#mainMenu")
    menu.find("a").each(function () {
        $(this).removeClass("active");
        $(this).removeClass("bg-blue");
        $(this).removeClass("bg-white");
    });
    // add active class to currently selected item
    current.addClass("active");
    current.addClass("bg-blue");
    // recursively add active class to parent menu items
    let menuId = menu.attr("id");
    while (current.attr("id") != menuId) {
        current = current.parent();
        if ($(current).attr("class").split(/\s+/).includes("menu")) {
            $(current).children("a").each(function () {
                $(this).addClass("active");
                $(this).addClass("bg-white");
            });
        }
    }
}
{{end}}
