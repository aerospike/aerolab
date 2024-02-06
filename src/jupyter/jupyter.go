package jupyter

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type NotebookType int

const (
	TypeBash  = NotebookType(1)
	TypeMagic = NotebookType(2)
)

type Jupyter struct {
	notebookType  NotebookType
	execCounter   int
	Cells         []*JupyterCell   `json:"cells"`          // one cell per history item
	Nbformat      int              `json:"nbformat"`       // 4
	NbformatMinor int              `json:"nbformat_minor"` // 5
	Metadata      *JupyterMetadata `json:"metadata"`       // static metadata
}

type JupyterCell struct {
	CellType       string        `json:"cell_type"`       // "code"
	ExecutionCount *int          `json:"execution_count"` // 1
	Id             string        `json:"id"`              // UUID.UUID string
	Metadata       struct{}      `json:"metadata"`        // empty struct
	Outputs        []interface{} `json:"outputs"`         // add JupyterOutput for stdout and JupyterError for error
	Source         []string      `json:"source"`          // one entry only: add the execution line
}

type JupyterError struct {
	Ename      string   `json:"ename"`       // fill with error name
	Evalue     string   `json:"evalue"`      // fill with string value of errorcode (like "1")
	OutputType string   `json:"output_type"` // "error"
	Traceback  []string `json:"traceback"`   // leave empty
}

type JupyterOutput struct {
	Name       string   `json:"name"`        // "stdout"
	OutputType string   `json:"output_type"` // "stream"
	Text       []string `json:"text"`        // one per line, each one containing the trailing \n
}

type JupyterMetadata struct {
	KernelSpec   *JupyterMetadataKernelSpec   `json:"kernelspec"`
	LanguageInfo *JupyterMetadataLanguageInfo `json:"language_info"`
}

type JupyterMetadataKernelSpec struct {
	DisplayName string `json:"display_name"` // "Bash"
	Language    string `json:"language"`     // "bash"
	Name        string `json:"name"`         // "bash"
}

type JupyterMetadataLanguageInfo struct {
	CodemirrorMode interface{} `json:"codemirror_mode"` // "shell" as string, or JupyterCodemirror
	FileExtension  string      `json:"file_extension"`  // ".sh"
	MimeType       string      `json:"mimetype"`        // "text/x-sh"
	Name           string      `json:"name"`            // "bash"
}

type JupyterCodemirror struct {
	Name    string `json:"name"`    // "ipython"
	Version int    `json:"version"` // 3
}

func New(notebookType NotebookType) *Jupyter {
	switch notebookType {
	case TypeMagic:
		return &Jupyter{
			notebookType:  notebookType,
			Nbformat:      4,
			NbformatMinor: 5,
			Metadata: &JupyterMetadata{
				KernelSpec: &JupyterMetadataKernelSpec{
					DisplayName: "AeroLab (iPython)",
					Language:    "python",
					Name:        "python3",
				},
				LanguageInfo: &JupyterMetadataLanguageInfo{
					CodemirrorMode: &JupyterCodemirror{
						Name:    "ipython",
						Version: 3,
					},
					FileExtension: ".py",
					MimeType:      "text/x-python",
					Name:          "python",
				},
			},
			Cells: []*JupyterCell{
				{
					CellType:       "code",
					ExecutionCount: nil,
					Metadata:       struct{}{},
					Id:             uuid.NewString(),
					Source:         []string{"# run this cell to enable horizontal scrolling\n", "from IPython.display import display, HTML\n", "display(HTML(\"<style>pre { white-space: pre !important; }</style>\"))"},
					Outputs:        []interface{}{},
				},
			},
		}
	case TypeBash:
		fallthrough
	default:
		return &Jupyter{
			notebookType:  notebookType,
			Nbformat:      4,
			NbformatMinor: 5,
			Metadata: &JupyterMetadata{
				KernelSpec: &JupyterMetadataKernelSpec{
					DisplayName: "AeroLab (Bash)",
					Language:    "bash",
					Name:        "bash",
				},
				LanguageInfo: &JupyterMetadataLanguageInfo{
					CodemirrorMode: "shell",
					FileExtension:  ".sh",
					MimeType:       "text/x-sh",
					Name:           "bash",
				},
			},
		}
	}
}

func (j *Jupyter) AddCell(command string, stdout string, errorCode int, errorName string) {
	outputText := strings.Split(stdout, "\n")
	for i := range outputText {
		outputText[i] = outputText[i] + "\n" // this is inefficient
	}
	if j.notebookType == TypeMagic {
		command = "!" + command
	}
	j.execCounter++
	ecount := j.execCounter
	cell := &JupyterCell{
		CellType:       "code",
		ExecutionCount: &ecount,
		Metadata:       struct{}{},
		Id:             uuid.NewString(),
		Source:         []string{command},
	}
	cell.Outputs = append(cell.Outputs, &JupyterOutput{
		Name:       "stdout",
		OutputType: "stream",
		Text:       outputText,
	})
	if errorCode > 0 {
		cell.Outputs = append(cell.Outputs, &JupyterError{
			OutputType: "error",
			Traceback:  []string{},
			Evalue:     strconv.Itoa(errorCode),
			Ename:      "",
		})
	}
	j.Cells = append(j.Cells, cell)
}
