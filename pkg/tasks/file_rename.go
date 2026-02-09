package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xbapps/xbvr/pkg/config"
	"github.com/xbapps/xbvr/pkg/models"
)

// VRIdentifiers holds extracted VR-related identifiers from a filename
type VRIdentifiers struct {
	Resolution string
	VRFormat   string
	Alpha      string
}

// RenamePreview represents the result of a rename preview
type RenamePreview struct {
	FileID       uint            `json:"file_id"`
	OldFilename  string          `json:"old_filename"`
	NewFilename  string          `json:"new_filename"`
	CanRename    bool            `json:"can_rename"`
	Reason       string          `json:"reason,omitempty"`
	RelatedFiles []RenamePreview `json:"related_files,omitempty"`
}

// RenameResult represents the result of a rename operation
type RenameResult struct {
	FileID      uint   `json:"file_id"`
	OldFilename string `json:"old_filename"`
	NewFilename string `json:"new_filename"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

// GenerateFilename generates a new filename based on the pattern template
// Pattern tokens: {Site}, {Studio}, {Title}, {Actors}, {FirstActor}, {Date}, {Year},
//
//	{Resolution}, {VRFormat}, {Alpha}, {SceneID}, {ext}
func GenerateFilename(file models.File, scene models.Scene, pattern string) (string, error) {
	log.Debugf("File Rename: Generating filename for file ID=%d scene=%s", file.ID, scene.SceneID)

	if pattern == "" {
		pattern = config.Config.FileRename.Pattern
	}
	if pattern == "" {
		pattern = "{Site} - {Title} - {Actors} - {Resolution}{VRFormat}{Alpha}.{ext}"
	}
	log.Debugf("File Rename: Using pattern: %s", pattern)

	// Extract VR identifiers from original filename
	vrIds := extractVRIdentifiers(file.Filename)
	log.Debugf("File Rename: Extracted VR identifiers - Resolution=%s VRFormat=%s Alpha=%s", vrIds.Resolution, vrIds.VRFormat, vrIds.Alpha)

	// If resolution wasn't found in filename, try to get it from video metadata
	if vrIds.Resolution == "" && file.VideoWidth > 0 && file.VideoHeight > 0 {
		vrIds.Resolution = GetResolutionFromVideo(file.VideoWidth, file.VideoHeight)
	}

	// Get file extension
	ext := strings.TrimPrefix(filepath.Ext(file.Filename), ".")

	// Build actor string
	actorSeparator := config.Config.FileRename.ActorSeparator
	if actorSeparator == "" {
		actorSeparator = ", "
	}
	maxActors := config.Config.FileRename.MaxActors
	if maxActors == 0 {
		maxActors = 3
	}

	var actorNames []string
	for i, actor := range scene.Cast {
		if i >= maxActors {
			break
		}
		actorNames = append(actorNames, actor.Name)
	}
	actors := strings.Join(actorNames, actorSeparator)

	firstActor := ""
	if len(scene.Cast) > 0 {
		firstActor = scene.Cast[0].Name
	}

	// Build date strings
	dateStr := ""
	yearStr := ""
	if !scene.ReleaseDate.IsZero() {
		dateStr = scene.ReleaseDate.Format("2006-01-02")
		yearStr = scene.ReleaseDate.Format("2006")
	}

	// Build resolution string with format suffix
	resolutionStr := vrIds.Resolution
	vrFormatStr := ""
	if vrIds.VRFormat != "" {
		vrFormatStr = "_" + vrIds.VRFormat
	}
	alphaStr := ""
	if vrIds.Alpha != "" {
		alphaStr = "_" + vrIds.Alpha
	}

	// Replace tokens
	result := pattern
	result = strings.ReplaceAll(result, "{Site}", scene.Site)
	result = strings.ReplaceAll(result, "{Studio}", scene.Studio)
	result = strings.ReplaceAll(result, "{Title}", scene.Title)
	result = strings.ReplaceAll(result, "{Actors}", actors)
	result = strings.ReplaceAll(result, "{FirstActor}", firstActor)
	result = strings.ReplaceAll(result, "{Date}", dateStr)
	result = strings.ReplaceAll(result, "{Year}", yearStr)
	result = strings.ReplaceAll(result, "{Resolution}", resolutionStr)
	result = strings.ReplaceAll(result, "{VRFormat}", vrFormatStr)
	result = strings.ReplaceAll(result, "{Alpha}", alphaStr)
	result = strings.ReplaceAll(result, "{SceneID}", scene.SceneID)
	result = strings.ReplaceAll(result, "{ext}", ext)

	// Sanitize the filename
	result = sanitizeFilename(result, config.Config.FileRename.LinuxFriendly)
	log.Debugf("File Rename: Sanitized filename (linux=%v): %s", config.Config.FileRename.LinuxFriendly, result)

	// Truncate if too long (max 255 chars)
	if len(result) > 255 {
		log.Debugf("File Rename: Truncating filename from %d to 255 chars", len(result))
		// Keep extension
		extWithDot := "." + ext
		maxBase := 255 - len(extWithDot)
		base := strings.TrimSuffix(result, extWithDot)
		if len(base) > maxBase {
			base = base[:maxBase]
		}
		result = base + extWithDot
	}

	return result, nil
}

// extractVRIdentifiers extracts resolution, VR format, and alpha from a filename
func extractVRIdentifiers(filename string) VRIdentifiers {
	ids := VRIdentifiers{}

	// Use the same separator pattern as volume.go
	filenameSeparator := regexp.MustCompile(`[ _.\-]`)
	nameParts := filenameSeparator.Split(strings.ToLower(filename), -1)

	// Extract VR format
	for i, part := range nameParts {
		switch part {
		case "mkx200", "mkx220", "rf52", "fisheye190", "vrca220", "flat":
			ids.VRFormat = part
		case "fisheye", "f180", "180f":
			ids.VRFormat = "fisheye"
		case "mono":
			if i < len(nameParts)-1 {
				next := nameParts[i+1]
				if next == "360" || next == "180" {
					ids.VRFormat = next + "_mono"
				}
			}
		case "360", "180":
			if i < len(nameParts)-1 && nameParts[i+1] == "mono" {
				ids.VRFormat = part + "_mono"
			} else if i > 0 && nameParts[i-1] == "mono" {
				// Already handled
			} else if part == "180" && ids.VRFormat == "" {
				// Check for 180_sbs pattern
				if i < len(nameParts)-1 && nameParts[i+1] == "sbs" {
					ids.VRFormat = "180_sbs"
				}
			} else if part == "360" && ids.VRFormat == "" {
				// Check for 360_tb pattern
				if i < len(nameParts)-1 && nameParts[i+1] == "tb" {
					ids.VRFormat = "360_tb"
				}
			}
		}
	}

	// Extract alpha (only valid for fisheye projections)
	if ids.VRFormat == "mkx200" || ids.VRFormat == "mkx220" || ids.VRFormat == "rf52" ||
		ids.VRFormat == "fisheye190" || ids.VRFormat == "vrca220" || ids.VRFormat == "fisheye" {
		for _, part := range nameParts {
			if part == "alpha" {
				ids.Alpha = "alpha"
				break
			}
		}
	}

	// Extract resolution
	// Look for patterns like 5k, 6k, 8k, 4k, or pixel dimensions like 5400x2700
	resolutionPatterns := map[string]string{
		"8k":        "8K",
		"7k":        "7K",
		"6k":        "6K",
		"5k":        "5K",
		"4k":        "4K",
		"uhd":       "4K",
		"7680x3840": "8K",
		"6144x3072": "6K",
		"5760x2880": "6K",
		"5400x2700": "5K",
		"4320x2160": "4K",
		"4096x2048": "4K",
		"3840x1920": "4K",
		"2880x1440": "3K",
	}

	for _, part := range nameParts {
		if res, ok := resolutionPatterns[part]; ok {
			ids.Resolution = res
			break
		}
	}

	// If still no resolution, check for resolution in combined format (e.g., "5K" as part of a larger token)
	if ids.Resolution == "" {
		lowerFilename := strings.ToLower(filename)
		resRegex := regexp.MustCompile(`\b([4-8])k\b`)
		if match := resRegex.FindStringSubmatch(lowerFilename); len(match) > 1 {
			ids.Resolution = strings.ToUpper(match[1]) + "K"
		}
	}

	// If no resolution found from filename, leave it empty (could be calculated from video metadata)
	return ids
}

// sanitizeFilename removes or replaces invalid characters in a filename
// Follows the pattern from GetFunscriptTitle()
func sanitizeFilename(name string, linuxFriendly bool) string {
	if linuxFriendly {
		// Linux/shell-friendly: replace spaces and remove shell-problematic characters
		// Characters that cause issues in bash/zsh: spaces, quotes, backticks, $, &, |, ;, (), [], {}, *, ?, #, !, \

		// Replace spaces, colons, and dashes with underscores
		name = strings.ReplaceAll(name, " ", "_")
		name = strings.ReplaceAll(name, ":", "_")
		name = strings.ReplaceAll(name, "-", "_")

		// Remove shell-problematic characters
		shellUnsafe := regexp.MustCompile(`['"` + "`" + `$&|;()\[\]{}*?#!\\<>]`)
		name = shellUnsafe.ReplaceAllString(name, "")

		// Clean up multiple underscores
		multiUnderscore := regexp.MustCompile(`_+`)
		name = multiUnderscore.ReplaceAllString(name, "_")

		// Remove leading/trailing underscores
		name = strings.Trim(name, "_")
	} else {
		// Standard sanitization (Windows-compatible)
		// Replace colons with unicode alternative (common in titles)
		name = strings.ReplaceAll(name, ":", "꞉")

		// Remove other unsafe characters
		re := regexp.MustCompile(`[?/\\<>|"*]`)
		name = re.ReplaceAllString(name, "")

		// Clean up multiple spaces
		spaceRe := regexp.MustCompile(`\s+`)
		name = spaceRe.ReplaceAllString(name, " ")

		// Trim leading/trailing spaces
		name = strings.TrimSpace(name)
	}

	return name
}

// resolveConflict appends a number suffix if the filename already exists
func resolveConflict(basePath, newFilename string) string {
	fullPath := filepath.Join(basePath, newFilename)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return newFilename
	}

	// File exists, need to find a unique name
	ext := filepath.Ext(newFilename)
	base := strings.TrimSuffix(newFilename, ext)

	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		candidatePath := filepath.Join(basePath, candidate)
		if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
			return candidate
		}
	}

	// Fallback (shouldn't happen)
	return newFilename
}

// findRelatedFiles finds related files (.funscript, .srt, .hsp, .ssa, .ass) for a given video file
func findRelatedFiles(file models.File) []models.File {
	db, _ := models.GetDB()
	defer db.Close()

	var relatedFiles []models.File

	// Get the base filename without extension
	baseName := strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))

	// Look for related files in the same directory
	relatedExtensions := []string{".funscript", ".srt", ".hsp", ".ssa", ".ass", ".cmscript"}

	for _, ext := range relatedExtensions {
		relatedFilename := baseName + ext
		var related models.File
		err := db.Preload("Volume").Where("path = ? AND filename = ?", file.Path, relatedFilename).First(&related).Error
		if err == nil {
			relatedFiles = append(relatedFiles, related)
		}
	}

	return relatedFiles
}

// performRename performs the actual rename operation for a file
func performRename(file models.File, newFilename string) error {
	switch file.Volume.Type {
	case "local":
		oldPath := filepath.Join(file.Path, file.Filename)
		newPath := filepath.Join(file.Path, newFilename)

		// Check if new file already exists
		if _, err := os.Stat(newPath); err == nil {
			return fmt.Errorf("file already exists: %s", newFilename)
		}

		err := os.Rename(oldPath, newPath)
		if err != nil {
			return fmt.Errorf("failed to rename file: %w", err)
		}

	case "putio":
		client := file.Volume.GetPutIOClient()
		id, err := strconv.ParseInt(file.Path, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid Put.io file ID: %w", err)
		}
		err = client.Files.Rename(context.Background(), id, newFilename)
		if err != nil {
			return fmt.Errorf("failed to rename Put.io file: %w", err)
		}

	default:
		return fmt.Errorf("unsupported volume type: %s", file.Volume.Type)
	}

	return nil
}

// updateSceneFilenames updates the scene's FilenamesArr after a rename
func updateSceneFilenames(scene *models.Scene, oldFilename, newFilename string) error {
	var filenames []string
	if err := json.Unmarshal([]byte(scene.FilenamesArr), &filenames); err != nil {
		// If unmarshal fails, start with empty array
		filenames = []string{}
	}

	// Replace old filename with new one
	found := false
	for i, fn := range filenames {
		if fn == oldFilename {
			filenames[i] = newFilename
			found = true
			break
		}
	}

	// If not found, add the new filename
	if !found {
		filenames = append(filenames, newFilename)
	}

	data, err := json.Marshal(filenames)
	if err != nil {
		return err
	}

	scene.FilenamesArr = string(data)
	return nil
}

// PreviewRename generates a preview of what a file would be renamed to
func PreviewRename(fileID uint) (*RenamePreview, error) {
	log.Debugf("File Rename: Generating preview for file ID=%d", fileID)

	db, _ := models.GetDB()
	defer db.Close()

	var file models.File
	err := db.Preload("Volume").Where(&models.File{ID: fileID}).First(&file).Error
	if err != nil {
		log.Errorf("File Rename: File not found ID=%d: %v", fileID, err)
		return nil, fmt.Errorf("file not found: %w", err)
	}

	preview := &RenamePreview{
		FileID:      file.ID,
		OldFilename: file.Filename,
	}

	// Check if file is matched to a scene
	if file.SceneID == 0 {
		log.Debugf("File Rename: File ID=%d not matched to scene", fileID)
		preview.CanRename = false
		preview.Reason = "File is not matched to a scene"
		return preview, nil
	}

	var scene models.Scene
	err = db.Preload("Cast").Where(&models.Scene{ID: file.SceneID}).First(&scene).Error
	if err != nil {
		preview.CanRename = false
		preview.Reason = "Scene not found"
		return preview, nil
	}

	// Generate new filename
	newFilename, err := GenerateFilename(file, scene, "")
	if err != nil {
		preview.CanRename = false
		preview.Reason = fmt.Sprintf("Failed to generate filename: %v", err)
		return preview, nil
	}

	// Resolve conflicts for local files
	if file.Volume.Type == "local" {
		newFilename = resolveConflict(file.Path, newFilename)
	}

	preview.NewFilename = newFilename

	// Check if rename is needed
	if preview.OldFilename == preview.NewFilename {
		preview.CanRename = false
		preview.Reason = "Filename unchanged"
		return preview, nil
	}

	preview.CanRename = true

	// Find related files
	relatedFiles := findRelatedFiles(file)
	for _, related := range relatedFiles {
		oldExt := filepath.Ext(related.Filename)
		newBase := strings.TrimSuffix(newFilename, filepath.Ext(newFilename))
		relatedNewFilename := newBase + oldExt

		relatedPreview := RenamePreview{
			FileID:      related.ID,
			OldFilename: related.Filename,
			NewFilename: relatedNewFilename,
			CanRename:   true,
		}
		preview.RelatedFiles = append(preview.RelatedFiles, relatedPreview)
	}

	return preview, nil
}

// PreviewBatchRename generates previews for multiple files
func PreviewBatchRename(fileIDs []uint) ([]RenamePreview, error) {
	var previews []RenamePreview

	for _, fileID := range fileIDs {
		preview, err := PreviewRename(fileID)
		if err != nil {
			previews = append(previews, RenamePreview{
				FileID:    fileID,
				CanRename: false,
				Reason:    err.Error(),
			})
			continue
		}
		previews = append(previews, *preview)
	}

	return previews, nil
}

// RenameFile performs a single file rename operation
func RenameFile(fileID uint) (*RenameResult, error) {
	log.Infof("File Rename: Starting rename for file ID=%d", fileID)

	db, _ := models.GetDB()
	defer db.Close()

	var file models.File
	err := db.Preload("Volume").Where(&models.File{ID: fileID}).First(&file).Error
	if err != nil {
		log.Errorf("File Rename: File not found ID=%d: %v", fileID, err)
		return nil, fmt.Errorf("file not found: %w", err)
	}
	log.Debugf("File Rename: Loaded file '%s' from volume type=%s", file.Filename, file.Volume.Type)

	result := &RenameResult{
		FileID:      file.ID,
		OldFilename: file.Filename,
	}

	// Check if file is matched to a scene
	if file.SceneID == 0 {
		log.Debugf("File Rename: File ID=%d not matched to scene", fileID)
		result.Success = false
		result.Error = "File is not matched to a scene"
		return result, nil
	}

	var scene models.Scene
	err = db.Preload("Cast").Where(&models.Scene{ID: file.SceneID}).First(&scene).Error
	if err != nil {
		result.Success = false
		result.Error = "Scene not found"
		return result, nil
	}

	// Generate new filename
	newFilename, err := GenerateFilename(file, scene, "")
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Failed to generate filename: %v", err)
		return result, nil
	}

	// Resolve conflicts for local files
	if file.Volume.Type == "local" {
		newFilename = resolveConflict(file.Path, newFilename)
	}

	result.NewFilename = newFilename

	// Check if rename is needed
	if file.Filename == newFilename {
		result.Success = true
		return result, nil
	}

	// Find related files before rename
	relatedFiles := findRelatedFiles(file)

	// Perform the rename
	log.Debugf("File Rename: Performing rename '%s' -> '%s'", file.Filename, newFilename)
	err = performRename(file, newFilename)
	if err != nil {
		log.Errorf("File Rename: Failed to rename file ID=%d: %v", file.ID, err)
		result.Success = false
		result.Error = err.Error()
		return result, nil
	}

	// Update file record in database
	oldFilename := file.Filename
	file.Filename = newFilename
	now := time.Now()
	file.RenamedAt = &now
	file.Save()
	log.Debugf("File Rename: Updated database record for file ID=%d", file.ID)

	// Update scene filenames
	updateSceneFilenames(&scene, oldFilename, newFilename)
	scene.Save()
	log.Debugf("File Rename: Updated scene filenames for scene ID=%d", scene.ID)

	// Rename related files
	log.Debugf("File Rename: Processing %d related files", len(relatedFiles))
	for _, related := range relatedFiles {
		oldExt := filepath.Ext(related.Filename)
		newBase := strings.TrimSuffix(newFilename, filepath.Ext(newFilename))
		relatedNewFilename := newBase + oldExt

		if related.Volume.Type == "local" {
			relatedNewFilename = resolveConflict(related.Path, relatedNewFilename)
		}

		err := performRename(related, relatedNewFilename)
		if err != nil {
			// Log error but continue with other files
			log.Warnf("Failed to rename related file %s: %v", related.Filename, err)
			continue
		}

		// Update related file record
		related.Filename = relatedNewFilename
		related.Save()
		log.Debugf("File Rename: Successfully renamed related file '%s' -> '%s'", related.Filename, relatedNewFilename)
	}

	result.Success = true
	log.Infof("File Rename: Successfully renamed '%s' -> '%s'", oldFilename, newFilename)
	return result, nil
}

// BatchRename performs rename operations on multiple files
func BatchRename(fileIDs []uint) ([]RenameResult, error) {
	log.Infof("File Rename: Starting batch rename for %d files", len(fileIDs))
	var results []RenameResult

	successCount := 0
	failCount := 0

	for i, fileID := range fileIDs {
		log.Debugf("File Rename: Batch progress %d/%d - processing file ID=%d", i+1, len(fileIDs), fileID)
		result, err := RenameFile(fileID)
		if err != nil {
			failCount++
			results = append(results, RenameResult{
				FileID:  fileID,
				Success: false,
				Error:   err.Error(),
			})
			continue
		}
		if result.Success {
			successCount++
		} else {
			failCount++
		}
		results = append(results, *result)
	}

	log.Infof("File Rename: Batch rename completed - %d successful, %d failed", successCount, failCount)
	return results, nil
}

// GetResolutionFromVideo calculates resolution label from video dimensions
func GetResolutionFromVideo(width, height int) string {
	// For VR videos, width is typically 2x height for 180 SBS
	effectiveHeight := height

	switch {
	case effectiveHeight >= 3840:
		return "8K"
	case effectiveHeight >= 3200:
		return "7K"
	case effectiveHeight >= 2880:
		return "6K"
	case effectiveHeight >= 2400:
		return "5K"
	case effectiveHeight >= 1920:
		return "4K"
	case effectiveHeight >= 1440:
		return "3K"
	case effectiveHeight >= 1080:
		return "1080p"
	default:
		return ""
	}
}
