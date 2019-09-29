package gphoto2go

// #cgo pkg-config: libgphoto2
// #include <gphoto2.h>
// #include <stdlib.h>
import "C"
import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"unsafe"
)

const (
	CAPTURE_IMAGE = C.GP_CAPTURE_IMAGE
	CAPTURE_MOVIE = C.GP_CAPTURE_MOVIE
	CAPTURE_SOUND = C.GP_CAPTURE_SOUND
)

type Camera struct {
	camera  *C.Camera
	context *C.GPContext
}
type CameraFile struct {
	file *C.CameraFile
}

type CameraFilePath struct {
	Name   string
	Folder string
}

type CameraWidget struct {
	widget *C.CameraWidget
}

func (c *Camera) Init() error {
	c.context = C.gp_context_new()

	C.gp_camera_new(&c.camera)
	return cameraResultToError(C.gp_camera_init(c.camera, c.context))
}

func (c *Camera) Exit() error {
	err := C.gp_camera_exit(c.camera, c.context)
	return cameraResultToError(err)
}

func (c *Camera) Cancel() {
	C.gp_context_cancel(c.context)
}

func (c *Camera) GetAbilities() (C.CameraAbilities, error) {
	var abilities C.CameraAbilities
	err := cameraResultToError(C.gp_camera_get_abilities(c.camera, &abilities))
	return abilities, err
}

func (c *Camera) TriggerCapture() error {
	err := C.gp_camera_trigger_capture(c.camera, c.context)
	return cameraResultToError(err)
}

func (c *Camera) TriggerCaptureToFile() (CameraFilePath, error) {
	var path CameraFilePath
	var _path C.CameraFilePath
	err := cameraResultToError(C.gp_camera_capture(c.camera, CAPTURE_IMAGE, &_path, c.context))
	if err != nil {
		return path, err
	}
	path.Name = C.GoString(&_path.name[0])
	path.Folder = C.GoString(&_path.folder[0])
	return path, nil
}

type CameraEventType int

const (
	EventUnknown   CameraEventType = C.GP_EVENT_UNKNOWN
	EventTimeout   CameraEventType = C.GP_EVENT_TIMEOUT
	EventFileAdded CameraEventType = C.GP_EVENT_FILE_ADDED
)

type CameraEvent struct {
	Type   CameraEventType
	Folder string
	File   string
}

func (c *Camera) AsyncWaitForEvent(timeout int) chan *CameraEvent {
	var eventType C.CameraEventType
	var vp unsafe.Pointer
	defer C.free(vp)

	ch := make(chan *CameraEvent)

	go func() {
		C.gp_camera_wait_for_event(c.camera, C.int(timeout), &eventType, &vp, c.context)
		ch <- cCameraEventToGoCameraEvent(vp, eventType)
	}()

	return ch
}

func cCameraEventToGoCameraEvent(voidPtr unsafe.Pointer, eventType C.CameraEventType) *CameraEvent {
	ce := new(CameraEvent)
	ce.Type = CameraEventType(eventType)

	if ce.Type == EventFileAdded {
		cameraFilePath := (*C.CameraFilePath)(voidPtr)
		ce.File = C.GoString((*C.char)(&cameraFilePath.name[0]))
		ce.Folder = C.GoString((*C.char)(&cameraFilePath.folder[0]))
	}

	return ce
}

func (c *Camera) ListFolders(folder string) ([]string, error) {
	if folder == "" {
		folder = "/"
	}

	var cameraList *C.CameraList
	C.gp_list_new(&cameraList)
	defer C.free(unsafe.Pointer(cameraList))

	cFolder := C.CString(folder)
	defer C.free(unsafe.Pointer(cFolder))

	if err := cameraResultToError(C.gp_camera_folder_list_folders(c.camera, cFolder, cameraList, c.context)); err != nil {
		return []string{}, err
	}
	folderMap, _ := cameraListToMap(cameraList)

	names := make([]string, len(folderMap))
	i := 0
	for key, _ := range folderMap {
		names[i] = key
		i += 1
	}

	return names, nil
}

func (c *Camera) RListFolders(folder string) []string {
	folders := make([]string, 0)
	path := folder
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	subfolders, _ := c.ListFolders(path)
	for _, sub := range subfolders {
		subPath := path + sub
		folders = append(folders, subPath)
		folders = append(folders, c.RListFolders(subPath)...)
	}

	return folders
}

func (c *Camera) ListFiles(folder string) ([]string, error) {
	if folder == "" {
		folder = "/"
	}

	if !strings.HasSuffix(folder, "/") {
		folder = folder + "/"
	}

	var cameraList *C.CameraList
	C.gp_list_new(&cameraList)
	defer C.free(unsafe.Pointer(cameraList))

	cFolder := C.CString(folder)
	defer C.free(unsafe.Pointer(cFolder))

	if err := cameraResultToError(C.gp_camera_folder_list_files(c.camera, cFolder, cameraList, c.context)); err != nil {
		return []string{}, err
	}
	fileNameMap, _ := cameraListToMap(cameraList)

	names := make([]string, len(fileNameMap))
	i := 0
	for key, _ := range fileNameMap {
		names[i] = key
		i += 1
	}

	return names, nil
}

func cameraListToMap(cameraList *C.CameraList) (map[string]string, int) {
	size := int(C.gp_list_count(cameraList))
	vals := make(map[string]string)

	if size < 0 {
		return vals, size
	}

	for i := 0; i < size; i++ {
		var cKey *C.char
		var cVal *C.char

		C.gp_list_get_name(cameraList, C.int(i), &cKey)
		C.gp_list_get_value(cameraList, C.int(i), &cVal)
		defer C.free(unsafe.Pointer(cKey))
		defer C.free(unsafe.Pointer(cVal))
		key := C.GoString(cKey)
		val := C.GoString(cVal)

		vals[key] = val
	}

	return vals, 0
}

func (c *Camera) Model() (string, error) {
	abilities, err := c.GetAbilities()
	if err != nil {
		return "", err
	}
	model := C.GoString((*C.char)(&abilities.model[0]))

	return model, nil
}

func cameraResultToError(err C.int) error {
	if err != 0 {
		return fmt.Errorf(C.GoString(C.gp_result_as_string(err)))
	}
	return nil
}

func CameraResultToString(err C.int) string {
	return C.GoString(C.gp_result_as_string(err))
}

// Need to find a good buffer size
// For now, let's try 1MB
const fileReaderBufferSize = 1 * 1024 * 1024

type cameraFileReader struct {
	camera   *Camera
	folder   string
	fileName string
	fullSize uint64
	offset   uint64
	closed   bool

	cCameraFile *C.CameraFile
	cBuffer     *C.char

	buffer [fileReaderBufferSize]byte
}

func (cfr *cameraFileReader) Read(p []byte) (int, error) {
	if cfr.closed {
		return 0, io.ErrClosedPipe
	}

	n := uint64(len(p))

	if n == 0 {
		return 0, nil
	}

	bufLen := uint64(len(cfr.buffer))
	remaining := cfr.fullSize - cfr.offset

	toRead := bufLen
	if toRead > remaining {
		toRead = remaining
	}

	if toRead > n {
		toRead = n
	}

	// From: https://code.google.com/p/go-wiki/wiki/cgo
	// Turning C arrays into Go slices
	sliceHeader := reflect.SliceHeader{
		Data: uintptr(unsafe.Pointer(cfr.cBuffer)),
		Len:  int(cfr.fullSize),
		Cap:  int(cfr.fullSize),
	}
	goSlice := *(*[]C.char)(unsafe.Pointer(&sliceHeader))

	for i := uint64(0); i < toRead; i++ {
		p[i] = byte(goSlice[cfr.offset+i])
	}

	cfr.offset += toRead

	if cfr.offset < cfr.fullSize {
		return int(toRead), nil
	}
	return int(toRead), io.EOF
}

func (cfr *cameraFileReader) Close() error {
	if !cfr.closed {
		// If I understand correctly, freeing the CameraFile will also free the data buffer (ie. cfr.cBuffer)
		C.gp_file_free(cfr.cCameraFile)
		cfr.closed = true
	}
	return nil
}

func (c *Camera) FileReader(folder string, fileName string) io.ReadCloser {
	cfr := new(cameraFileReader)
	cfr.camera = c
	cfr.folder = folder
	cfr.fileName = fileName
	cfr.offset = 0
	cfr.closed = false

	cFileName := C.CString(cfr.fileName)
	cFolderName := C.CString(cfr.folder)
	defer C.free(unsafe.Pointer(cFileName))
	defer C.free(unsafe.Pointer(cFolderName))

	C.gp_file_new(&cfr.cCameraFile)
	C.gp_camera_file_get(c.camera, cFolderName, cFileName, C.GP_FILE_TYPE_NORMAL, cfr.cCameraFile, c.context)

	var cSize C.ulong
	C.gp_file_get_data_and_size(cfr.cCameraFile, &cfr.cBuffer, &cSize)

	cfr.fullSize = uint64(cSize)

	return cfr
}

func (c *Camera) DeleteFile(folder, file string) error {
	folderBytes := []byte(folder)
	fileBytes := []byte(file)
	//Convert the byte arrays into C pointers

	folderPointer := (*C.char)(unsafe.Pointer(&folderBytes[0]))
	filePointer := (*C.char)(unsafe.Pointer(&fileBytes[0]))
	return cameraResultToError(C.gp_camera_file_delete(c.camera, folderPointer, filePointer, c.context))
}

func (c *Camera) CapturePreview() (cf CameraFile, err error) {
	C.gp_file_new(&cf.file)
	err = cameraResultToError(C.gp_camera_capture_preview(
		c.camera,
		cf.file,
		c.context))
	getPreviewFile(&cf)
	return cf, err

}
func getPreviewFile(file *CameraFile) {
	var cSize C.ulong
	var buf *C.char
	C.gp_file_get_data_and_size(file.file, &buf, &cSize)
}

func (w *CameraWidget) Free() {
	if err := cameraResultToError(C.gp_widget_free(w.widget)); err != nil {
		fmt.Printf("WARNING: error on C.gp_widget_free: %v\n", err)
	}
}

func (w *CameraWidget) GetChildrenByName(name string) (*CameraWidget, error) {
	var child *C.CameraWidget

	n := C.CString(name)
	defer C.free(unsafe.Pointer(n))

	err := cameraResultToError(C.gp_widget_get_child_by_name(w.widget, n, &child))
	if err != nil {
		C.free(unsafe.Pointer(child))
		return nil, fmt.Errorf("error on C.gp_widget_get_child_by_name(%s): %v", name, err)
	}

	return &CameraWidget{child}, nil
}

func (w *CameraWidget) SetValue(v interface{}) error {
	switch v.(type) {
	case string:
		cstr := C.CString(v.(string))
		defer C.free(unsafe.Pointer(cstr))

		if err := cameraResultToError(C.gp_widget_set_value(w.widget, unsafe.Pointer(cstr))); err != nil {
			return err
		}
	default:
		if err := cameraResultToError(C.gp_widget_set_value(w.widget, unsafe.Pointer(&v))); err != nil {
			return err
		}
	}

	return nil
}

func (c *Camera) SetConfig(w *CameraWidget) error {
	if err := cameraResultToError(C.gp_camera_set_config(c.camera, w.widget, c.context)); err != nil {
		return fmt.Errorf("error on C.gp_camera_set_config %v", err)
	}
	return nil
}

func (c *Camera) GetConfig() (*CameraWidget, error) {
	w := CameraWidget{}

	p := C.CString("")
	defer C.free(unsafe.Pointer(p))

	err := cameraResultToError(C.gp_widget_new(C.GP_WIDGET_WINDOW, p, &w.widget))
	if err != nil {
		C.free(unsafe.Pointer(w.widget))
		return nil, err
	}

	if err := cameraResultToError(C.gp_camera_get_config(c.camera, &w.widget, c.context)); err != nil {
		w.Free()
		return nil, err
	}
	return &w, nil
}
