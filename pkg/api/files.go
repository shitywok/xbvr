package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/markphelps/optional"
	"github.com/xbapps/xbvr/pkg/models"
	"github.com/xbapps/xbvr/pkg/tasks"
)

type RequestMatchFile struct {
	SceneID string `json:"scene_id"`
	FileID  uint   `json:"file_id"`
}

type RequestUnmatchFile struct {
	FileID uint `json:"file_id"`
}

type RequestFileList struct {
	State       optional.String   `json:"state"`
	CreatedDate []optional.String `json:"createdDate"`
	Sort        optional.String   `json:"sort"`
	Resolutions []optional.String `json:"resolutions"`
	Framerates  []optional.String `json:"framerates"`
	Bitrates    []optional.String `json:"bitrates"`
	Filename    optional.String   `json:"filename"`
}

type RequestRenamePreview struct {
	FileID uint `json:"file_id"`
}

type RequestRenameBatch struct {
	FileIDs []uint `json:"file_ids"`
}

type RequestFileManagerList struct {
	Limit         optional.Int      `json:"limit"`
	Offset        optional.Int      `json:"offset"`
	Sites         []optional.String `json:"sites"`
	Cast          []optional.String `json:"cast"`
	Resolutions   []optional.String `json:"resolutions"`
	RenamedStatus optional.String   `json:"renamedStatus"` // "all", "renamed", "not_renamed"
	MatchedOnly   optional.Bool     `json:"matchedOnly"`
	SavedSearchID optional.Int      `json:"savedSearchId"`
}

type FileManagerItem struct {
	File        models.File `json:"file"`
	Scene       *SceneInfo  `json:"scene"`
	NewFilename string      `json:"new_filename"`
}

type SceneInfo struct {
	ID       uint     `json:"id"`
	Title    string   `json:"title"`
	Site     string   `json:"site"`
	Cast     []string `json:"cast"`
	Released string   `json:"released"`
}

type FileManagerResponse struct {
	Files []FileManagerItem `json:"files"`
	Total int               `json:"total"`
}

type FilesResource struct{}

func (i FilesResource) WebService() *restful.WebService {
	tags := []string{"Files"}

	ws := new(restful.WebService)

	ws.Path("/api/files").
		Consumes(restful.MIME_JSON).
		Produces(restful.MIME_JSON)

	ws.Route(ws.POST("/list").To(i.listFiles).
		Metadata(restfulspec.KeyOpenAPITags, tags))

	ws.Route(ws.POST("/match").To(i.matchFile).
		Metadata(restfulspec.KeyOpenAPITags, tags))

	ws.Route(ws.POST("/unmatch").To(i.unmatchFile).
		Metadata(restfulspec.KeyOpenAPITags, tags))

	ws.Route(ws.GET("/file/{file-id}").To(i.getFile).
		Param(ws.PathParameter("file-id", "File ID").DataType("int")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(models.File{}))

	ws.Route(ws.DELETE("/file/{file-id}").To(i.removeFile).
		Metadata(restfulspec.KeyOpenAPITags, tags))

	ws.Route(ws.POST("/rename/preview").To(i.previewRename).
		Metadata(restfulspec.KeyOpenAPITags, tags))

	ws.Route(ws.POST("/rename/preview-batch").To(i.previewBatchRename).
		Metadata(restfulspec.KeyOpenAPITags, tags))

	ws.Route(ws.POST("/rename").To(i.renameFile).
		Metadata(restfulspec.KeyOpenAPITags, tags))

	ws.Route(ws.POST("/rename-batch").To(i.batchRename).
		Metadata(restfulspec.KeyOpenAPITags, tags))

	ws.Route(ws.POST("/manager/list").To(i.fileManagerList).
		Metadata(restfulspec.KeyOpenAPITags, tags))

	ws.Route(ws.GET("/manager/filters").To(i.fileManagerFilters).
		Metadata(restfulspec.KeyOpenAPITags, tags))

	return ws
}

func (i FilesResource) getFile(req *restful.Request, resp *restful.Response) {
	var file models.File

	id, err := strconv.Atoi(req.PathParameter("file-id"))
	if err != nil {
		log.Error(err)
		return
	}

	_ = file.GetIfExistByPK(uint(id))

	resp.WriteHeaderAndEntity(http.StatusOK, file)
}

func (i FilesResource) listFiles(req *restful.Request, resp *restful.Response) {
	db, _ := models.GetDB()
	defer db.Close()

	var r RequestFileList
	err := req.ReadEntity(&r)
	if err != nil {
		log.Error(err)
		return
	}

	var files []models.File
	tx := db.Model(&files)

	// State
	switch r.State.OrElse("") {
	case "matched":
		tx = tx.Where("files.scene_id != 0")
	case "unmatched":
		tx = tx.Where("files.scene_id = 0")
	}

	// Resolution
	resolutionClauses := []string{}
	if len(r.Resolutions) > 0 {
		for _, resolution := range r.Resolutions {
			if resolution.OrElse("") == "below4k" {
				resolutionClauses = append(resolutionClauses, "video_height between 0 and 1899")
			}
			if resolution.OrElse("") == "4k" {
				resolutionClauses = append(resolutionClauses, "video_height between 1900 and 2449")
			}
			if resolution.OrElse("") == "5k" {
				resolutionClauses = append(resolutionClauses, "video_height between 2450 and 2899")
			}
			if resolution.OrElse("") == "6k" {
				resolutionClauses = append(resolutionClauses, "video_height between 2900 and 3299")
			}
			if resolution.OrElse("") == "above6k" {
				resolutionClauses = append(resolutionClauses, "video_height between 3300 and 9999")
			}
		}
		tx = tx.Where("(" + strings.Join(resolutionClauses, " OR ") + ") AND video_height != 0")
	}

	// Bitrate
	bitrateClauses := []string{}
	if len(r.Bitrates) > 0 {
		for _, bitrate := range r.Bitrates {
			if bitrate.OrElse("") == "low" {
				bitrateClauses = append(bitrateClauses, "video_bit_rate between 0 and 14999999")
			}
			if bitrate.OrElse("") == "medium" {
				bitrateClauses = append(bitrateClauses, "video_bit_rate between 15000000 and 24999999")
			}
			if bitrate.OrElse("") == "high" {
				bitrateClauses = append(bitrateClauses, "video_bit_rate between 25000000 and 35000000")
			}
			if bitrate.OrElse("") == "ultra" {
				bitrateClauses = append(bitrateClauses, "video_bit_rate between 35000001 and 999999999")
			}
		}
		tx = tx.Where("(" + strings.Join(bitrateClauses, " OR ") + ") AND video_bit_rate != 0")
	}

	// Framerate
	framerateClauses := []string{}
	if len(r.Framerates) > 0 {
		for _, framerate := range r.Framerates {
			if framerate.OrElse("") == "30fps" {
				framerateClauses = append(framerateClauses, "video_avg_frame_rate_val = 30.0")
			}
			if framerate.OrElse("") == "60fps" {
				framerateClauses = append(framerateClauses, "video_avg_frame_rate_val = 60.0")
			}
			if framerate.OrElse("") == "other" {
				framerateClauses = append(framerateClauses, "(video_avg_frame_rate_val != 30.0 AND video_avg_frame_rate_val != 60.0)")
			}
		}
		tx = tx.Where("(" + strings.Join(framerateClauses, " OR ") + ") AND video_avg_frame_rate_val != 0")
	}

	// Filename
	if len(r.Filename.OrElse("")) > 0 {
		tx = tx.Where("filename like ?", "%"+r.Filename.OrElse("")+"%")
	}

	// Creation date
	if len(r.CreatedDate) == 2 {
		t0, _ := time.Parse(time.RFC3339, r.CreatedDate[0].OrElse(""))
		t1, _ := time.Parse(time.RFC3339, r.CreatedDate[1].OrElse(""))
		tx = tx.Where("files.created_time > ? AND files.created_time < ?", t0, t1)
	}

	// Sorting
	switch r.Sort.OrElse("") {
	case "filename_asc":
		tx = tx.Order("filename asc")
	case "filename_desc":
		tx = tx.Order("filename desc")
	case "created_time_asc":
		tx = tx.Order("created_time asc")
	case "created_time_desc":
		tx = tx.Order("created_time desc")
	case "duration_asc":
		tx = tx.Order("video_duration asc")
	case "duration_desc":
		tx = tx.Order("video_duration desc")
	case "size_asc":
		tx = tx.Order("size asc")
	case "size_desc":
		tx = tx.Order("size desc")
	case "video_height_asc":
		tx = tx.Order("video_height asc")
	case "video_height_desc":
		tx = tx.Order("video_height desc")
	case "video_width_asc":
		tx = tx.Order("video_width asc")
	case "video_width_desc":
		tx = tx.Order("video_width desc")
	case "video_bitrate_asc":
		tx = tx.Order("video_bit_rate asc")
	case "video_bitrate_desc":
		tx = tx.Order("video_bit_rate desc")
	case "video_avgfps_val_asc":
		tx = tx.Order("video_avg_frame_rate_val asc")
	case "video_avgfps_val_desc":
		tx = tx.Order("video_avg_frame_rate_val desc")
	}

	tx.Find(&files)

	resp.WriteHeaderAndEntity(http.StatusOK, files)
}

func (i FilesResource) matchFile(req *restful.Request, resp *restful.Response) {
	db, _ := models.GetDB()
	defer db.Close()

	var r RequestMatchFile
	err := req.ReadEntity(&r)
	if err != nil {
		log.Error(err)
		return
	}

	// Assign Scene to File
	var scene models.Scene
	err = scene.GetIfExist(r.SceneID)
	if err != nil {
		log.Error(err)
		return
	}

	var f models.File
	err = db.Preload("Volume").Where(&models.File{ID: r.FileID}).First(&f).Error
	if err == nil {
		f.SceneID = scene.ID
		f.Save()
	}

	// Add File to the list of Scene filenames so it will be discovered when file is moved
	var pfTxt []string
	err = json.Unmarshal([]byte(scene.FilenamesArr), &pfTxt)
	if err != nil {
		log.Error(err)
		return
	}

	pfTxt = append(pfTxt, f.Filename)
	tmp, err := json.Marshal(pfTxt)
	if err == nil {
		scene.FilenamesArr = string(tmp)
	}

	models.AddAction(scene.SceneID, "match", "filenames_arr", scene.FilenamesArr)

	// Finally, update scene available/accessible status
	scene.UpdateStatus()

	resp.WriteHeaderAndEntity(http.StatusOK, nil)
}

func (i FilesResource) unmatchFile(req *restful.Request, resp *restful.Response) {
	db, _ := models.GetDB()
	defer db.Close()

	var r RequestUnmatchFile
	err := req.ReadEntity(&r)
	if err != nil {
		log.Error(err)
		return
	}

	var f models.File
	err = db.Preload("Volume").Where(&models.File{ID: r.FileID}).First(&f).Error
	var sceneID uint = 0
	if err == nil {
		sceneID = f.SceneID
		if sceneID != 0 {
			f.SceneID = 0
			f.Save()
		}

	}

	var scene models.Scene
	if sceneID != 0 {
		err = scene.GetIfExistByPK(sceneID)
		if err != nil {
			log.Error(err)
			return
		}

		// Remove File from the list of Scene filenames so it will be not be auto-matched again
		var pfTxt []string
		err = json.Unmarshal([]byte(scene.FilenamesArr), &pfTxt)
		if err != nil {
			log.Error(err)
			return
		}

		var newFilenamesArr []string

		for _, fn := range pfTxt {
			if fn != f.Filename {
				newFilenamesArr = append(newFilenamesArr, fn)
			}
		}

		tmp, err := json.Marshal(newFilenamesArr)
		if err == nil {
			scene.FilenamesArr = string(tmp)
		}

		models.AddAction(scene.SceneID, "unmatch", "filenames_arr", scene.FilenamesArr)

		// Finally, update scene available/accessible status
		scene.UpdateStatus()
	}

	resp.WriteHeaderAndEntity(http.StatusOK, scene)
}

func (i FilesResource) removeFile(req *restful.Request, resp *restful.Response) {
	fileId, err := strconv.Atoi(req.PathParameter("file-id"))
	if err != nil {
		return
	}
	scene := removeFileByFileId(uint(fileId))
	resp.WriteHeaderAndEntity(http.StatusOK, scene)
}
func removeFileByFileId(fileId uint) models.Scene {

	var scene models.Scene
	var file models.File
	db, _ := models.GetDB()
	defer db.Close()

	err := db.Preload("Volume").Where(&models.File{ID: fileId}).First(&file).Error
	if err == nil {

		log.Infof("Deleting file %s", filepath.Join(file.Path, file.Filename))
		deleted := false
		switch file.Volume.Type {
		case "local":
			err := os.Remove(filepath.Join(file.Path, file.Filename))
			if err == nil || errors.Is(err, fs.ErrNotExist) {
				deleted = true
			} else {
				log.Errorf("error deleting file: %v", err)
			}
		case "putio":
			id, err := strconv.ParseInt(file.Path, 10, 64)
			if err != nil {
				return scene
			}
			client := file.Volume.GetPutIOClient()
			err = client.Files.Delete(context.Background(), id)
			if err == nil {
				deleted = true
			} else {
				log.Errorf("error deleting file %v", err)
			}
		}

		if deleted {
			db.Delete(&file)
			if file.SceneID != 0 {
				scene.GetIfExistByPK(file.SceneID)
				scene.UpdateStatus()
			}
		}
	} else {
		log.Errorf("error deleting file %v", err)
	}
	return scene
}

func (i FilesResource) previewRename(req *restful.Request, resp *restful.Response) {
	var r RequestRenamePreview
	err := req.ReadEntity(&r)
	if err != nil {
		log.Errorf("File Manager: Preview rename failed to parse request: %v", err)
		resp.WriteHeaderAndEntity(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	log.Debugf("File Manager: Previewing rename for file ID=%d", r.FileID)
	preview, err := tasks.PreviewRename(r.FileID)
	if err != nil {
		log.Errorf("File Manager: Preview rename failed for file ID=%d: %v", r.FileID, err)
		resp.WriteHeaderAndEntity(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Debugf("File Manager: Preview generated - old='%s' new='%s'", preview.OldFilename, preview.NewFilename)
	resp.WriteHeaderAndEntity(http.StatusOK, preview)
}

func (i FilesResource) previewBatchRename(req *restful.Request, resp *restful.Response) {
	var r RequestRenameBatch
	err := req.ReadEntity(&r)
	if err != nil {
		log.Errorf("File Manager: Batch preview failed to parse request: %v", err)
		resp.WriteHeaderAndEntity(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	log.Infof("File Manager: Previewing batch rename for %d files", len(r.FileIDs))
	previews, err := tasks.PreviewBatchRename(r.FileIDs)
	if err != nil {
		log.Errorf("File Manager: Batch preview failed: %v", err)
		resp.WriteHeaderAndEntity(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Infof("File Manager: Batch preview completed for %d files", len(previews))
	resp.WriteHeaderAndEntity(http.StatusOK, previews)
}

func (i FilesResource) renameFile(req *restful.Request, resp *restful.Response) {
	var r RequestRenamePreview
	err := req.ReadEntity(&r)
	if err != nil {
		log.Errorf("File Manager: Rename failed to parse request: %v", err)
		resp.WriteHeaderAndEntity(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	log.Infof("File Manager: Renaming file ID=%d", r.FileID)
	result, err := tasks.RenameFile(r.FileID)
	if err != nil {
		log.Errorf("File Manager: Rename failed for file ID=%d: %v", r.FileID, err)
		resp.WriteHeaderAndEntity(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if result.Success {
		log.Infof("File Manager: Successfully renamed '%s' -> '%s'", result.OldFilename, result.NewFilename)
	} else {
		log.Errorf("File Manager: Rename unsuccessful for file ID=%d: %s", r.FileID, result.Error)
	}
	resp.WriteHeaderAndEntity(http.StatusOK, result)
}

func (i FilesResource) batchRename(req *restful.Request, resp *restful.Response) {
	var r RequestRenameBatch
	err := req.ReadEntity(&r)
	if err != nil {
		log.Errorf("File Manager: Batch rename failed to parse request: %v", err)
		resp.WriteHeaderAndEntity(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	log.Infof("File Manager: Starting batch rename for %d files", len(r.FileIDs))
	results, err := tasks.BatchRename(r.FileIDs)
	if err != nil {
		log.Errorf("File Manager: Batch rename failed: %v", err)
		resp.WriteHeaderAndEntity(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	successCount := 0
	failCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failCount++
		}
	}
	log.Infof("File Manager: Batch rename completed - %d successful, %d failed", successCount, failCount)
	resp.WriteHeaderAndEntity(http.StatusOK, results)
}

func (i FilesResource) fileManagerList(req *restful.Request, resp *restful.Response) {
	log.Info("File Manager: Loading file list")

	var r RequestFileManagerList
	err := req.ReadEntity(&r)
	if err != nil {
		log.Errorf("File Manager: Failed to parse request: %v", err)
		resp.WriteHeaderAndEntity(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Log filter parameters at debug level
	log.Debugf("File Manager: Filters - status=%s, sites=%d, cast=%d, resolutions=%d, savedSearchId=%v",
		r.RenamedStatus.OrElse("all"),
		len(r.Sites),
		len(r.Cast),
		len(r.Resolutions),
		r.SavedSearchID.OrElse(0))

	db, _ := models.GetDB()
	defer db.Close()

	// Base query - only matched files (files with scene_id > 0)
	query := db.Model(&models.File{}).
		Preload("Volume").
		Where("scene_id > 0")

	// Apply renamed status filter
	if status, err := r.RenamedStatus.Get(); err == nil {
		log.Debugf("File Manager: Applying status filter: %s", status)
		switch status {
		case "renamed":
			query = query.Where("renamed_at IS NOT NULL")
		case "not_renamed":
			query = query.Where("renamed_at IS NULL")
		}
	}

	// Apply site filter
	if len(r.Sites) > 0 {
		var siteNames []string
		for _, s := range r.Sites {
			if siteVal, err := s.Get(); err == nil {
				siteNames = append(siteNames, siteVal)
			}
		}
		if len(siteNames) > 0 {
			log.Debugf("File Manager: Filtering by sites: %v", siteNames)
			query = query.Where("scene_id IN (SELECT id FROM scenes WHERE site IN (?))", siteNames)
		}
	}

	// Apply cast filter
	if len(r.Cast) > 0 {
		var castNames []string
		for _, c := range r.Cast {
			if castVal, err := c.Get(); err == nil {
				castNames = append(castNames, castVal)
			}
		}
		if len(castNames) > 0 {
			log.Debugf("File Manager: Filtering by cast: %v", castNames)
			query = query.Where("scene_id IN (SELECT scene_id FROM scene_cast WHERE actor_id IN (SELECT id FROM actors WHERE name IN (?)))", castNames)
		}
	}

	// Apply resolution filter
	if len(r.Resolutions) > 0 {
		var resConditions []string
		for _, res := range r.Resolutions {
			if resVal, err := res.Get(); err == nil {
				switch resVal {
				case "8K":
					resConditions = append(resConditions, "video_width >= 7680")
				case "6K":
					resConditions = append(resConditions, "(video_width >= 5760 AND video_width < 7680)")
				case "5K":
					resConditions = append(resConditions, "(video_width >= 4800 AND video_width < 5760)")
				case "4K":
					resConditions = append(resConditions, "(video_width >= 3840 AND video_width < 4800)")
				}
			}
		}
		if len(resConditions) > 0 {
			log.Debugf("File Manager: Filtering by resolutions: %v", resConditions)
			query = query.Where("(" + strings.Join(resConditions, " OR ") + ")")
		}
	}

	// Apply saved search filter
	if savedSearchID, err := r.SavedSearchID.Get(); err == nil && savedSearchID > 0 {
		log.Debugf("File Manager: Applying saved search filter ID=%d", savedSearchID)
		var playlist models.Playlist
		if err := db.First(&playlist, savedSearchID).Error; err == nil && playlist.IsSmart {
			log.Debugf("File Manager: Found saved search '%s'", playlist.Name)
			// Parse saved search params
			var searchParams models.RequestSceneList
			if err := json.Unmarshal([]byte(playlist.SearchParams), &searchParams); err == nil {
				// Query scenes matching the saved search (without pagination)
				searchParams.Limit = optional.NewInt(10000)
				searchParams.Offset = optional.NewInt(0)
				result := models.QueryScenes(searchParams, false)

				// Extract scene IDs
				var sceneIDs []uint
				for _, scene := range result.Scenes {
					sceneIDs = append(sceneIDs, scene.ID)
				}

				log.Debugf("File Manager: Saved search matched %d scenes", len(sceneIDs))
				if len(sceneIDs) > 0 {
					query = query.Where("scene_id IN (?)", sceneIDs)
				} else {
					// No matching scenes, return empty result
					query = query.Where("1 = 0")
				}
			} else {
				log.Errorf("File Manager: Failed to parse saved search params: %v", err)
			}
		} else if err != nil {
			log.Errorf("File Manager: Failed to load saved search ID=%d: %v", savedSearchID, err)
		}
	}

	// Get total count before pagination
	var total int
	query.Count(&total)

	// Apply pagination
	limit := 50
	if l, err := r.Limit.Get(); err == nil && l > 0 {
		limit = l
	}
	offset := 0
	if o, err := r.Offset.Get(); err == nil {
		offset = o
	}

	var files []models.File
	query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&files)

	log.Debugf("File Manager: Query returned %d files (total: %d, limit: %d, offset: %d)", len(files), total, limit, offset)

	// Build response with scene info and preview filenames
	var items []FileManagerItem
	for _, file := range files {
		item := FileManagerItem{
			File: file,
		}

		// Get scene info
		if file.SceneID > 0 {
			var scene models.Scene
			if err := db.Preload("Cast").Where("id = ?", file.SceneID).First(&scene).Error; err == nil {
				var castNames []string
				for _, actor := range scene.Cast {
					castNames = append(castNames, actor.Name)
				}
				item.Scene = &SceneInfo{
					ID:       scene.ID,
					Title:    scene.Title,
					Site:     scene.Site,
					Cast:     castNames,
					Released: scene.ReleaseDate.Format("2006-01-02"),
				}

				// Generate preview filename
				if preview, err := tasks.PreviewRename(file.ID); err == nil {
					item.NewFilename = preview.NewFilename
				} else {
					log.Debugf("File Manager: Failed to generate preview for file %d: %v", file.ID, err)
				}
			}
		}

		items = append(items, item)
	}

	response := FileManagerResponse{
		Files: items,
		Total: total,
	}

	log.Infof("File Manager: Returning %d files (total matching: %d)", len(items), total)
	resp.WriteHeaderAndEntity(http.StatusOK, response)
}

func (i FilesResource) fileManagerFilters(req *restful.Request, resp *restful.Response) {
	log.Info("File Manager: Loading filters")

	db, _ := models.GetDB()
	defer db.Close()

	// Get unique sites from matched files
	var sites []string
	db.Raw("SELECT DISTINCT site FROM scenes WHERE id IN (SELECT scene_id FROM files WHERE scene_id > 0) ORDER BY site").Pluck("site", &sites)
	log.Debugf("File Manager: Found %d sites", len(sites))

	// Get unique cast from matched files
	var cast []string
	db.Raw("SELECT DISTINCT a.name FROM actors a INNER JOIN scene_cast sc ON a.id = sc.actor_id WHERE sc.scene_id IN (SELECT scene_id FROM files WHERE scene_id > 0) ORDER BY a.name").Pluck("name", &cast)
	log.Debugf("File Manager: Found %d cast members", len(cast))

	// Get saved searches (smart playlists for scenes)
	type SavedSearchInfo struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	var savedSearches []SavedSearchInfo
	db.Model(&models.Playlist{}).
		Where("is_smart = ? AND playlist_type = ?", true, "scene").
		Order("ordering asc, name asc").
		Select("id, name").
		Scan(&savedSearches)
	log.Debugf("File Manager: Found %d saved searches", len(savedSearches))

	// Get resolution counts
	type ResolutionCount struct {
		Label string `json:"label"`
		Count int    `json:"count"`
	}
	var resolutions []ResolutionCount

	var count8K, count6K, count5K, count4K int
	db.Model(&models.File{}).Where("scene_id > 0 AND video_width >= 7680").Count(&count8K)
	db.Model(&models.File{}).Where("scene_id > 0 AND video_width >= 5760 AND video_width < 7680").Count(&count6K)
	db.Model(&models.File{}).Where("scene_id > 0 AND video_width >= 4800 AND video_width < 5760").Count(&count5K)
	db.Model(&models.File{}).Where("scene_id > 0 AND video_width >= 3840 AND video_width < 4800").Count(&count4K)

	resolutions = append(resolutions, ResolutionCount{"8K", count8K})
	resolutions = append(resolutions, ResolutionCount{"6K", count6K})
	resolutions = append(resolutions, ResolutionCount{"5K", count5K})
	resolutions = append(resolutions, ResolutionCount{"4K", count4K})
	log.Debugf("File Manager: Resolution counts - 8K:%d, 6K:%d, 5K:%d, 4K:%d", count8K, count6K, count5K, count4K)

	// Get renamed status counts
	var renamedCount, notRenamedCount, totalMatched int
	db.Model(&models.File{}).Where("scene_id > 0 AND renamed_at IS NOT NULL").Count(&renamedCount)
	db.Model(&models.File{}).Where("scene_id > 0 AND renamed_at IS NULL").Count(&notRenamedCount)
	db.Model(&models.File{}).Where("scene_id > 0").Count(&totalMatched)
	log.Debugf("File Manager: Stats - total:%d, renamed:%d, not_renamed:%d", totalMatched, renamedCount, notRenamedCount)

	resp.WriteHeaderAndEntity(http.StatusOK, map[string]any{
		"sites":         sites,
		"cast":          cast,
		"savedSearches": savedSearches,
		"resolutions":   resolutions,
		"stats": map[string]int{
			"total":       totalMatched,
			"renamed":     renamedCount,
			"not_renamed": notRenamedCount,
		},
	})
}
