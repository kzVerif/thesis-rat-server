package service

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const MaxUploadSize = 100 << 20

const (
	FilesManagePermission     = "files.manage"
	FilesUploadPermission     = "files.upload"
	FilesDeletePermission     = "files.delete"
	FilesDistributePermission = "files.distribute"
)

type FileStore struct {
	db  *sql.DB
	dir string
}

type fileRecord struct {
	ID           string    `json:"id"`
	Filename     string    `json:"filename"`
	OriginalName string    `json:"original_name"`
	FileSize     int64     `json:"file_size"`
	HashSHA256   string    `json:"hash_sha256"`
	UploadedBy   string    `json:"uploaded_by"`
	Uploader     string    `json:"uploader"`
	CreatedAt    time.Time `json:"created_at"`
	ContentType  string    `json:"content_type"`
	Extension    string    `json:"extension"`
}

func NewFileStore(db *sql.DB, dir string) (*FileStore, error) {
	if db == nil {
		return nil, errors.New("จำเป็นต้องกำหนดฐานข้อมูลสำหรับระบบไฟล์")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("ไม่สามารถระบุตำแหน่งโฟลเดอร์อัปโหลดได้: %w", err)
	}
	if err := os.MkdirAll(abs, 0750); err != nil {
		return nil, fmt.Errorf("ไม่สามารถสร้างโฟลเดอร์อัปโหลดได้: %w", err)
	}
	return &FileStore{db: db, dir: abs}, nil
}

func validFilename(name string) bool {
	return validateFilename(name) == nil
}

// validateFilename enforces a portable filename that is safe on common
// Windows and POSIX filesystems while still allowing Unicode names.
func validateFilename(name string) error {
	if name == "" {
		return errors.New("กรุณาระบุชื่อไฟล์")
	}
	if len(name) > 255 {
		return errors.New("ชื่อไฟล์ต้องมีขนาดไม่เกิน 255 ไบต์")
	}
	if !utf8.ValidString(name) {
		return errors.New("ชื่อไฟล์ต้องเป็นข้อความ UTF-8 ที่ถูกต้อง")
	}
	if name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return errors.New("ชื่อไฟล์ต้องไม่มี path หรือเครื่องหมายแบ่งโฟลเดอร์")
	}
	if strings.TrimSpace(name) != name {
		return errors.New("ชื่อไฟล์ต้องไม่ขึ้นต้นหรือลงท้ายด้วยช่องว่าง")
	}
	if strings.HasPrefix(name, ".") {
		return errors.New("ชื่อไฟล์ต้องไม่ขึ้นต้นด้วยจุด")
	}
	if strings.HasSuffix(name, ".") {
		return errors.New("ชื่อไฟล์ต้องไม่ลงท้ายด้วยจุด")
	}
	if strings.ContainsAny(name, `<>:"|?*`) {
		return errors.New(`ชื่อไฟล์มีอักขระต้องห้าม: < > : " | ? *`)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("ชื่อไฟล์ต้องไม่มีอักขระควบคุม")
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	reserved := base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CLOCK$" ||
		(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9')
	if reserved {
		return errors.New("ชื่อไฟล์เป็นชื่อที่ระบบสงวนไว้")
	}
	return nil
}

func uniqueFilename(original string) string {
	ext := filepath.Ext(original)
	base := strings.TrimSuffix(original, ext)
	maxBase := 255 - len(ext) - 37
	if maxBase < 1 {
		ext, maxBase = "", 218
	}
	if len(base) > maxBase {
		base = base[:maxBase]
		for !utf8.ValidString(base) {
			base = base[:len(base)-1]
		}
	}
	return base + "-" + uuid.NewString() + ext
}

func (s *FileStore) createUnique(original string) (*os.File, string, error) {
	for i := 0; i < 10; i++ {
		name := uniqueFilename(original)
		f, err := os.OpenFile(filepath.Join(s.dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0640)
		if err == nil {
			return f, name, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", errors.New("ไม่สามารถสร้างชื่อไฟล์ที่ไม่ซ้ำได้")
}

// storageFilePath supports legacy filename-only rows while ensuring that both
// legacy and absolute paths resolve inside the configured upload directory.
func (s *FileStore) storageFilePath(storagePath string) (string, error) {
	if storagePath == "" {
		return "", errors.New("empty storage path")
	}
	resolved := storagePath
	if !filepath.IsAbs(resolved) {
		if !validFilename(resolved) {
			return "", errors.New("invalid legacy storage path")
		}
		resolved = filepath.Join(s.dir, resolved)
	}
	resolved = filepath.Clean(resolved)
	relative, err := filepath.Rel(s.dir, resolved)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", errors.New("storage path is outside upload directory")
	}
	return resolved, nil
}

func multipartReader(c *fiber.Ctx) (*multipart.Reader, error) {
	mediaType, params, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
	if err != nil || mediaType != fiber.MIMEMultipartForm {
		return nil, errors.New("Content-Type ต้องเป็น multipart/form-data")
	}
	if params["boundary"] == "" {
		return nil, errors.New("ไม่พบ multipart boundary")
	}
	reader := c.Context().RequestBodyStream()
	if reader == nil {
		reader = bytes.NewReader(c.Body())
	}
	return multipart.NewReader(reader, params["boundary"]), nil
}

func contentTypeFor(filename string) string {
	value := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if value == "" {
		return "application/octet-stream"
	}
	return value
}

func validateRenameExtension(current, requested string) error {
	if filepath.Ext(requested) != filepath.Ext(current) {
		return errors.New("ไม่อนุญาตให้เปลี่ยนนามสกุลไฟล์")
	}
	return nil
}

func scanFile(scanner interface{ Scan(...interface{}) error }) (fileRecord, error) {
	var item fileRecord
	err := scanner.Scan(&item.ID, &item.Filename, &item.OriginalName, &item.FileSize, &item.HashSHA256,
		&item.UploadedBy, &item.Uploader, &item.CreatedAt)
	item.Extension = filepath.Ext(item.Filename)
	item.ContentType = contentTypeFor(item.Filename)
	return item, err
}

const fileSelect = `SELECT f.id,f.filename,f.original_name,f.file_size,f.hash_sha256,
	f.uploaded_by,u.username,f.created_at FROM files f JOIN users u ON u.id=f.uploaded_by`

func (s *FileStore) List(c *fiber.Ctx) error {
	page, err := positiveQueryInt(c, "page", 1, 0)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "page ต้องเป็นจำนวนเต็มที่มากกว่า 0"})
	}
	limit, err := positiveQueryInt(c, "limit", 20, 100)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "limit ต้องเป็นจำนวนเต็มตั้งแต่ 1 ถึง 100"})
	}
	if int64(page-1) > int64(^uint64(0)>>1)/int64(limit) {
		return c.Status(400).JSON(fiber.Map{"error": "ค่า page มีขนาดใหญ่เกินไป"})
	}
	offset := int64(page-1) * int64(limit)
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&total); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการไฟล์ได้"})
	}
	rows, err := s.db.Query(fileSelect+` ORDER BY f.created_at DESC,f.id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการไฟล์ได้"})
	}
	defer rows.Close()
	items := make([]fileRecord, 0)
	for rows.Next() {
		item, err := scanFile(rows)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการไฟล์ได้"})
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการไฟล์ได้"})
	}
	totalPages := int64(0)
	if total > 0 {
		totalPages = (total + int64(limit) - 1) / int64(limit)
	}
	return c.JSON(fiber.Map{"files": items, "pagination": fiber.Map{
		"page": page, "limit": limit, "total": total, "total_pages": totalPages,
	}})
}

func (s *FileStore) Upload(c *fiber.Ctx) error {
	mr, err := multipartReader(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณาส่งไฟล์ใน multipart field ชื่อ 'file'"})
		}
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "ข้อมูล multipart ไม่ถูกต้อง"})
		}
		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		original := part.FileName()
		if err := validateFilename(original); err != nil {
			_ = part.Close()
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		out, storedName, err := s.createUnique(original)
		if err != nil {
			_ = part.Close()
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถจัดเก็บไฟล์ได้"})
		}
		path, keep := out.Name(), false
		defer func() {
			_ = out.Close()
			_ = part.Close()
			if !keep {
				_ = os.Remove(path)
			}
		}()
		hash := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(part, MaxUploadSize+1))
		if copyErr != nil || out.Sync() != nil || out.Close() != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถจัดเก็บไฟล์ได้"})
		}
		if written > MaxUploadSize {
			return c.Status(413).JSON(fiber.Map{"error": "ไฟล์มีขนาดเกินกำหนด 100 MiB"})
		}
		user := signedInUser(c)
		hashValue := hex.EncodeToString(hash.Sum(nil))
		item := fileRecord{
			Filename: storedName, OriginalName: original, FileSize: written, HashSHA256: hashValue,
			UploadedBy: user.ID, Uploader: user.Username, ContentType: contentTypeFor(storedName), Extension: filepath.Ext(storedName),
		}
		err = s.db.QueryRow(`INSERT INTO files(filename,original_name,file_size,storage_path,hash_sha256,uploaded_by)
			VALUES($1,$2,$3,$4,$5,$6) RETURNING id,created_at`, storedName, original, written, path,
			hashValue, user.ID).Scan(&item.ID, &item.CreatedAt)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถบันทึกข้อมูลไฟล์ลงฐานข้อมูลได้"})
		}
		keep = true
		return c.Status(201).JSON(fiber.Map{"message": "อัปโหลดไฟล์สำเร็จ", "file": item})
	}
}

func (s *FileStore) Rename(c *fiber.Ctx) error {
	current := c.Params("filename")
	if !validFilename(current) {
		return c.Status(400).JSON(fiber.Map{"error": "ชื่อไฟล์ไม่ถูกต้อง"})
	}
	var in struct {
		Name string `json:"name"`
	}
	if c.BodyParser(&in) != nil {
		return c.Status(400).JSON(fiber.Map{"error": "ข้อมูลที่ส่งมาไม่ถูกต้อง"})
	}
	if err := validateFilename(in.Name); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	currentExtension := filepath.Ext(current)
	requestedExtension := filepath.Ext(in.Name)
	if err := validateRenameExtension(current, in.Name); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error":               err.Error(),
			"current_extension":   currentExtension,
			"requested_extension": requestedExtension,
		})
	}
	tx, err := s.db.Begin()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถเปลี่ยนชื่อไฟล์ได้"})
	}
	defer tx.Rollback()
	var id, storagePath string
	err = tx.QueryRow(`SELECT id,storage_path FROM files WHERE filename=$1 FOR UPDATE`, current).Scan(&id, &storagePath)
	if errors.Is(err, sql.ErrNoRows) {
		return c.Status(404).JSON(fiber.Map{"error": "ไม่พบไฟล์"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถเปลี่ยนชื่อไฟล์ได้"})
	}
	newName := uniqueFilename(in.Name)
	oldPath, err := s.storageFilePath(storagePath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "invalid storage path"})
	}
	newPath := filepath.Join(s.dir, newName)
	if err := os.Rename(oldPath, newPath); errors.Is(err, os.ErrNotExist) {
		return c.Status(409).JSON(fiber.Map{"error": "พบข้อมูลไฟล์ในฐานข้อมูล แต่ไม่พบไฟล์ในพื้นที่จัดเก็บ"})
	} else if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถเปลี่ยนชื่อไฟล์ได้"})
	}
	if _, err = tx.Exec(`UPDATE files SET filename=$1,storage_path=$2 WHERE id=$3`, newName, newPath, id); err != nil {
		_ = os.Rename(newPath, oldPath)
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอัปเดตข้อมูลชื่อไฟล์ในฐานข้อมูลได้"})
	}
	item, err := scanFile(tx.QueryRow(fileSelect+` WHERE f.id=$1`, id))
	if err != nil || tx.Commit() != nil {
		_ = os.Rename(newPath, oldPath)
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอัปเดตข้อมูลชื่อไฟล์ในฐานข้อมูลได้"})
	}
	return c.JSON(fiber.Map{"message": "เปลี่ยนชื่อไฟล์สำเร็จ", "file": item})
}

func (s *FileStore) Delete(c *fiber.Ctx) error {
	name := c.Params("filename")
	if !validFilename(name) {
		return c.Status(400).JSON(fiber.Map{"error": "ชื่อไฟล์ไม่ถูกต้อง"})
	}
	tx, err := s.db.Begin()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถลบไฟล์ได้"})
	}
	defer tx.Rollback()
	var id, storagePath string
	err = tx.QueryRow(`SELECT id,storage_path FROM files WHERE filename=$1 FOR UPDATE`, name).Scan(&id, &storagePath)
	if errors.Is(err, sql.ErrNoRows) {
		return c.Status(404).JSON(fiber.Map{"error": "ไม่พบไฟล์"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถลบไฟล์ได้"})
	}
	originalPath, err := s.storageFilePath(storagePath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "invalid storage path"})
	}
	trashPath := filepath.Join(s.dir, ".delete-"+uuid.NewString())
	if err := os.Rename(originalPath, trashPath); errors.Is(err, os.ErrNotExist) {
		return c.Status(409).JSON(fiber.Map{"error": "พบข้อมูลไฟล์ในฐานข้อมูล แต่ไม่พบไฟล์ในพื้นที่จัดเก็บ"})
	} else if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถลบไฟล์ได้"})
	}
	if _, err := tx.Exec(`DELETE FROM files WHERE id=$1`, id); err != nil {
		_ = os.Rename(trashPath, originalPath)
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถลบข้อมูลไฟล์ออกจากฐานข้อมูลได้"})
	}
	if err := tx.Commit(); err != nil {
		_ = os.Rename(trashPath, originalPath)
		return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถลบข้อมูลไฟล์ออกจากฐานข้อมูลได้"})
	}
	_ = os.Remove(trashPath)
	return c.SendStatus(204)
}
