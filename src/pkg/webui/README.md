# WebUI Package

The WebUI package provides web-based user interface components and utilities for Aerolab's web interface. It includes website installation utilities and comprehensive data structures for rendering web pages, forms, and navigation elements.

## Key Features

- **Website Installation** - Extract and install embedded web assets
- **Page Rendering** - Comprehensive page structure definitions
- **Form Management** - Dynamic form generation and handling
- **Navigation System** - Hierarchical menu and navigation structures
- **Inventory Display** - Data structures for displaying cluster inventory
- **Theme Support** - Customizable themes and styling options

## Main Components

### Website Installation
- `InstallWebsite(dst string, website []byte) error` - Extract embedded website files to destination directory
- `Website []byte` - Embedded website archive (www.tgz)

### Page Structure
The `Page` struct provides a complete page definition including:
- Page metadata (title, navigation, menu)
- Form elements and inventory data
- Display options and user preferences
- Error handling and messaging

### Form System
Comprehensive form handling with support for:
- **Input Fields** - Text, password, file inputs with validation
- **Toggle Switches** - Boolean options with descriptions
- **Select Dropdowns** - Single and multi-select options
- **Separators** - Visual form section dividers

### Navigation System
Hierarchical navigation with:
- **Top Navigation** - Primary navigation links
- **Main Menu** - Sidebar menu with nested items
- **Active State Management** - Automatic active item detection
- **Badge Support** - Status indicators and counters

## Data Structures

### Page Configuration
```go
type Page struct {
    PageTitle                               string
    FixedFooter                             bool
    FixedNavbar                             bool
    PendingActionsShowAllUsersToggle        bool
    PendingActionsShowAllUsersToggleChecked bool
    WebRoot                                 string
    FormCommandTitle                        string
    IsForm                                  bool
    IsInventory                             bool
    IsError                                 bool
    ErrorString                             string
    ErrorTitle                              string
    Navigation                              *Nav
    Menu                                    *MainMenu
    FormItems                               []*FormItem
    Inventory                               map[string]*InventoryItem
    Backend                                 string
    BetaTag                                 bool
    CurrentUser                             string
    ShortSwitches                           bool
    ShowDefaults                            bool
    FormDownload                            bool
    ShowSimpleModeButton                    bool
    SimpleMode                              bool
    HideInventory                           HideInventory
}
```

### Form Elements
```go
type FormItem struct {
    Type      FormItemType
    Input     FormItemInput
    Toggle    FormItemToggle
    Select    FormItemSelect
    Separator FormItemSeparator
}
```

### Menu System
```go
type MenuItem struct {
    HasChildren   bool
    Icon          string
    Name          string
    Href          string
    IsActive      bool
    ActiveColor   string
    Badge         MenuItemBadge
    Items         MenuItems
    Tooltip       string
    DrawSeparator bool
}
```

## Usage Examples

### Website Installation

```go
import "github.com/aerospike/aerolab/pkg/webui"

// Install embedded website to destination directory
err := webui.InstallWebsite("/var/www/aerolab", webui.Website)
if err != nil {
    log.Fatal("Failed to install website:", err)
}
```

### Page Creation

```go
page := &webui.Page{
    PageTitle:   "Cluster Management",
    IsForm:      true,
    WebRoot:     "/aerolab",
    Backend:     "aws",
    CurrentUser: "admin",
    Navigation: &webui.Nav{
        Top: []*webui.NavTop{
            {Name: "Home", Href: "/", Target: "_self"},
            {Name: "Clusters", Href: "/clusters", Target: "_self"},
        },
    },
}
```

### Form Creation

```go
formItems := []*webui.FormItem{
    {
        Type: webui.FormItemType{Input: true},
        Input: webui.FormItemInput{
            Name:        "Cluster Name",
            Description: "Name for the new cluster",
            ID:          "cluster-name",
            Type:        "text",
            Required:    true,
        },
    },
    {
        Type: webui.FormItemType{Select: true},
        Select: webui.FormItemSelect{
            Name:        "Instance Type",
            Description: "EC2 instance type",
            ID:          "instance-type",
            Required:    true,
            Items: []*webui.FormItemSelectItem{
                {Name: "t3.micro", Value: "t3.micro", Selected: true},
                {Name: "t3.small", Value: "t3.small", Selected: false},
                {Name: "t3.medium", Value: "t3.medium", Selected: false},
            },
        },
    },
    {
        Type: webui.FormItemType{Toggle: true},
        Toggle: webui.FormItemToggle{
            Name:        "Enable Monitoring",
            Description: "Enable cluster monitoring",
            ID:          "enable-monitoring",
            On:          true,
        },
    },
}

page.FormItems = formItems
```

### Menu Configuration

```go
menu := &webui.MainMenu{
    Items: webui.MenuItems{
        {
            Icon:        "fas fa-tachometer-alt",
            Name:        "Dashboard",
            Href:        "/dashboard",
            ActiveColor: webui.ActiveColorBlue,
        },
        {
            Icon:        "fas fa-server",
            Name:        "Clusters",
            Href:        "/clusters",
            HasChildren: true,
            Items: webui.MenuItems{
                {Name: "List", Href: "/clusters/list"},
                {Name: "Create", Href: "/clusters/create"},
                {Name: "Templates", Href: "/clusters/templates"},
            },
        },
    },
}

// Set active menu item based on current path
menu.Items.Set("/clusters/create", "/aerolab")
```

### Inventory Display

```go
inventory := map[string]*webui.InventoryItem{
    "clusters": {
        Fields: []*webui.InventoryItemField{
            {Name: "name", FriendlyName: "Cluster Name", Backend: "aws"},
            {Name: "status", FriendlyName: "Status", Backend: "aws"},
            {Name: "nodes", FriendlyName: "Node Count", Backend: "aws"},
        },
    },
    "volumes": {
        Fields: []*webui.InventoryItemField{
            {Name: "id", FriendlyName: "Volume ID", Backend: "aws"},
            {Name: "size", FriendlyName: "Size (GB)", Backend: "aws"},
            {Name: "type", FriendlyName: "Type", Backend: "aws"},
        },
    },
}

page.Inventory = inventory
page.IsInventory = true
```

### Badge Configuration

```go
menuItem := &webui.MenuItem{
    Name: "Alerts",
    Href: "/alerts",
    Badge: webui.MenuItemBadge{
        Show: true,
        Type: webui.BadgeTypeDanger,
        Text: "3",
    },
}
```

## Constants and Enums

### Content Types
- `ContentTypeForm` - Form content type
- `ContentTypeTable` - Table content type

### Badge Types
- `BadgeTypeWarning` - Warning badge (yellow)
- `BadgeTypeSuccess` - Success badge (green)
- `BadgeTypeDanger` - Danger badge (red)

### Active Colors
- `ActiveColorWhite` - White background for active items
- `ActiveColorBlue` - Blue background for active items

### Cookie Names
- `TruncateTimestampCookieName` - Cookie for timestamp truncation preference

## Integration

The WebUI package integrates with:
- **Template Engines** - Provides data structures for HTML template rendering
- **HTTP Handlers** - Supplies page data for web request handling
- **Form Processing** - Enables dynamic form generation and validation
- **Asset Management** - Handles static asset installation and serving

## Customization

The package supports extensive customization through:
- **Theme Configuration** - Custom colors and styling
- **Menu Structure** - Flexible navigation hierarchies
- **Form Layouts** - Dynamic form element arrangement
- **Inventory Views** - Configurable data display options

## Security Features

- **Input Validation** - Form field validation and sanitization
- **XSS Prevention** - Safe HTML rendering practices
- **CSRF Protection** - Token-based form protection (when integrated)
- **User Context** - User-specific content and permissions
