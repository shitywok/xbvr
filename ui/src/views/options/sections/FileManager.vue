<template>
  <div class="content">
    <h3 class="title">{{ $t('File Manager') }}</h3>
    <p class="subtitle">{{ $t('Rename files based on scene metadata') }}</p>

    <!-- Settings Collapse -->
    <b-collapse class="card mb-4" animation="slide" :open="settingsOpen" @open="settingsOpen = true" @close="settingsOpen = false">
      <template #trigger="props">
        <div class="card-header" role="button">
          <p class="card-header-title">
            <b-icon icon="cog"></b-icon>
            <span class="ml-2">{{ $t('Rename Settings') }}</span>
          </p>
          <a class="card-header-icon">
            <b-icon :icon="props.open ? 'chevron-up' : 'chevron-down'"></b-icon>
          </a>
        </div>
      </template>
      <div class="card-content">
        <!-- Pattern Template -->
        <h5 class="title is-6">{{ $t('Pattern Template') }}</h5>
        <b-field :label="$t('Filename Pattern')">
          <b-input ref="patternInput" v-model="pattern"></b-input>
        </b-field>

        <div class="tokens-help">
          <label class="label is-small">{{ $t('Available Tokens') }} <small class="has-text-grey">({{ $t('click to insert') }})</small></label>
          <div class="tags">
            <span class="tag is-info is-light" v-for="token in tokens" :key="token.name" @click="insertToken(token.name)" style="cursor: pointer;">
              <b-tooltip :label="token.description" position="is-top">
                {{ token.name }}
              </b-tooltip>
            </span>
          </div>
        </div>

        <hr>

        <div class="columns">
          <div class="column is-4">
            <b-field :label="$t('Actor Separator')">
              <b-input v-model="actorSeparator" style="width: 100px;"></b-input>
            </b-field>
          </div>
          <div class="column is-4">
            <b-field :label="$t('Maximum Actors in Filename')">
              <b-slider v-model="maxActors" :min="1" :max="10" :step="1" :tooltip="true" style="max-width: 200px;"></b-slider>
            </b-field>
          </div>
          <div class="column is-4">
            <b-field>
              <b-checkbox v-model="linuxFriendly">
                {{ $t('Linux-friendly filenames') }}
              </b-checkbox>
            </b-field>
            <p class="help">{{ $t('Replaces spaces with underscores and removes shell-problematic characters') }}</p>
          </div>
        </div>

        <b-message v-if="linuxFriendly" type="is-info" size="is-small">
          {{ $t('Shell-unsafe delimiters are disabled. Use + . _ or similar safe characters for separators.') }}
        </b-message>

        <hr>

        <!-- Preview -->
        <div class="preview-section">
          <label class="label">{{ $t('Preview') }}</label>
          <div class="box has-background-light">
            <p><strong>{{ $t('Example Input') }}:</strong></p>
            <code class="is-block">SLR_StudioName_SceneTitle_1234_180x180_3dh_LR_5K.mp4</code>
            <p class="mt-3"><strong>{{ $t('Example Output') }}:</strong></p>
            <code class="is-block preview-output">{{ previewFilename }}</code>
          </div>
        </div>

        <hr>

        <div class="buttons">
          <b-button type="is-primary" @click="saveSettings" :loading="isSaving">
            {{ $t('Save') }}
          </b-button>
          <b-button type="is-light" @click="resetToDefaults">
            {{ $t('Reset to Defaults') }}
          </b-button>
        </div>
      </div>
    </b-collapse>

    <!-- Stats Bar -->
    <div class="level mb-4" v-if="stats">
      <div class="level-item has-text-centered">
        <div>
          <p class="heading">{{ $t('Total Matched') }}</p>
          <p class="title is-4">{{ stats.total }}</p>
        </div>
      </div>
      <div class="level-item has-text-centered">
        <div>
          <p class="heading">{{ $t('Renamed') }}</p>
          <p class="title is-4 has-text-success">{{ stats.renamed }}</p>
        </div>
      </div>
      <div class="level-item has-text-centered">
        <div>
          <p class="heading">{{ $t('Not Renamed') }}</p>
          <p class="title is-4 has-text-warning">{{ stats.not_renamed }}</p>
        </div>
      </div>
    </div>

    <!-- Filters -->
    <div class="box">
      <div class="columns is-multiline">
        <div class="column is-2">
          <b-field :label="$t('Status')">
            <b-select v-model="filters.renamedStatus" @input="loadFiles" expanded size="is-small">
              <option value="all">{{ $t('All') }}</option>
              <option value="not_renamed">{{ $t('Not Renamed') }}</option>
              <option value="renamed">{{ $t('Renamed') }}</option>
            </b-select>
          </b-field>
        </div>
        <div class="column is-3">
          <b-field :label="$t('Saved Search')">
            <b-select v-model="filters.savedSearchId" @input="loadFiles" expanded size="is-small">
              <option :value="null">{{ $t('All Files') }}</option>
              <option v-for="search in availableSavedSearches" :key="search.id" :value="search.id">
                {{ search.name }}
              </option>
            </b-select>
          </b-field>
        </div>
        <div class="column is-2">
          <b-field :label="$t('Site')">
            <b-taginput
              v-model="filters.sites"
              :data="filteredSites"
              autocomplete
              size="is-small"
              @typing="getFilteredSites"
              @input="loadFiles"
              :open-on-focus="true"
              placeholder="Type to search..."
            >
              <template slot-scope="props">{{ props.option }}</template>
              <template slot="empty">No matching sites</template>
            </b-taginput>
          </b-field>
        </div>
        <div class="column is-3">
          <b-field :label="$t('Cast')">
            <b-taginput
              v-model="filters.cast"
              :data="filteredCast"
              autocomplete
              size="is-small"
              @typing="getFilteredCast"
              @input="loadFiles"
              :open-on-focus="true"
              placeholder="Type to search..."
            >
              <template slot-scope="props">{{ props.option }}</template>
              <template slot="empty">No matching cast</template>
            </b-taginput>
          </b-field>
        </div>
        <div class="column is-2">
          <b-field :label="$t('Resolution')">
            <b-select v-model="filters.resolution" @input="loadFiles" expanded size="is-small">
              <option value="">{{ $t('All') }}</option>
              <option v-for="res in availableResolutions" :key="res.label" :value="res.label">
                {{ res.label }} ({{ res.count }})
              </option>
            </b-select>
          </b-field>
        </div>
      </div>
      <div class="columns">
        <div class="column has-text-right">
          <b-button type="is-primary" @click="renameSelected" :disabled="selectedFiles.length === 0" :loading="isRenaming">
            {{ $t('Rename') }} ({{ selectedFiles.length }})
          </b-button>
        </div>
      </div>
    </div>

    <!-- File List -->
    <b-table
      :data="files"
      :loading="isLoading"
      checkable
      :checked-rows.sync="selectedFiles"
      :is-row-checkable="(row) => row.file.renamed_at === null && row.new_filename && row.new_filename !== row.file.filename"
      paginated
      backend-pagination
      :total="totalFiles"
      :per-page="perPage"
      @page-change="onPageChange"
      striped
      hoverable
    >
      <b-table-column field="file.filename" :label="$t('Current Filename')" v-slot="props">
        <span class="filename">{{ props.row.file.filename }}</span>
      </b-table-column>

      <b-table-column field="new_filename" :label="$t('New Filename')" v-slot="props">
        <span class="filename has-text-info" v-if="props.row.new_filename && props.row.new_filename !== props.row.file.filename">
          {{ props.row.new_filename }}
        </span>
        <span class="has-text-success" v-else-if="props.row.file.renamed_at">
          <b-icon icon="check" size="is-small"></b-icon> {{ $t('Already renamed') }}
        </span>
        <span class="has-text-grey-light" v-else>{{ $t('No change needed') }}</span>
      </b-table-column>

      <b-table-column field="scene.site" :label="$t('Site')" width="140" v-slot="props">
        {{ props.row.scene ? props.row.scene.site : '-' }}
      </b-table-column>

      <b-table-column field="scene.cast" :label="$t('Cast')" width="200" v-slot="props">
        <span v-if="props.row.scene && props.row.scene.cast">
          {{ props.row.scene.cast.slice(0, 2).join(', ') }}
          <span v-if="props.row.scene.cast.length > 2" class="has-text-grey">+{{ props.row.scene.cast.length - 2 }}</span>
        </span>
        <span v-else>-</span>
      </b-table-column>

      <b-table-column :label="$t('Actions')" width="100" v-slot="props">
        <b-button
          size="is-small"
          type="is-primary"
          outlined
          @click="renameSingle(props.row)"
          :disabled="props.row.file.renamed_at !== null || !props.row.new_filename || props.row.new_filename === props.row.file.filename"
          :loading="renamingFileId === props.row.file.id"
        >
          {{ $t('Rename') }}
        </b-button>
      </b-table-column>

      <template #empty>
        <div class="has-text-centered py-4">
          {{ $t('No files matching your filters') }}
        </div>
      </template>
    </b-table>
  </div>
</template>

<script>
import ky from 'ky'

export default {
  name: 'FileManager',
  data () {
    return {
      // Settings
      settingsOpen: false,
      pattern: '{Site} - {Title} - {Actors} - {Resolution}{VRFormat}{Alpha}.{ext}',
      actorSeparator: ', ',
      maxActors: 3,
      linuxFriendly: false,
      isSaving: false,
      tokens: [
        { name: '{Site}', description: 'Site name (e.g., VirtualRealPorn)' },
        { name: '{Studio}', description: 'Studio name' },
        { name: '{Title}', description: 'Scene title' },
        { name: '{Actors}', description: 'Actor names (limited by Max Actors)' },
        { name: '{FirstActor}', description: 'First actor name only' },
        { name: '{Date}', description: 'Release date (YYYY-MM-DD)' },
        { name: '{Year}', description: 'Release year (YYYY)' },
        { name: '{Resolution}', description: 'Video resolution (4K, 5K, 6K, 8K)' },
        { name: '{VRFormat}', description: 'VR format (mkx200, fisheye, etc.)' },
        { name: '{Alpha}', description: 'Alpha channel indicator' },
        { name: '{SceneID}', description: 'Scene ID from source' },
        { name: '{ext}', description: 'File extension (mp4, mkv, etc.)' }
      ],
      exampleScene: {
        site: 'VirtualRealPorn',
        studio: 'VirtualRealPorn',
        title: 'Amazing Scene Title',
        actors: ['Jane Doe', 'John Smith', 'Alice Wonder'],
        date: '2024-01-15',
        year: '2024',
        sceneId: 'vrp-12345'
      },
      exampleVR: {
        resolution: '5K',
        vrFormat: 'mkx200',
        alpha: ''
      },
      // File list
      files: [],
      selectedFiles: [],
      totalFiles: 0,
      currentPage: 1,
      perPage: 50,
      isLoading: false,
      isRenaming: false,
      renamingFileId: null,
      filters: {
        renamedStatus: 'not_renamed',
        savedSearchId: null,
        sites: [],
        cast: [],
        resolution: ''
      },
      // Filter data
      availableSites: [],
      availableCast: [],
      availableSavedSearches: [],
      availableResolutions: [],
      filteredSites: [],
      filteredCast: [],
      stats: null
    }
  },
  computed: {
    previewFilename () {
      let result = this.pattern

      result = result.replace('{Site}', this.exampleScene.site)
      result = result.replace('{Studio}', this.exampleScene.studio)
      result = result.replace('{Title}', this.exampleScene.title)

      const actorsList = this.exampleScene.actors.slice(0, this.maxActors)
      result = result.replace('{Actors}', actorsList.join(this.actorSeparator))
      result = result.replace('{FirstActor}', this.exampleScene.actors[0] || '')

      result = result.replace('{Date}', this.exampleScene.date)
      result = result.replace('{Year}', this.exampleScene.year)
      result = result.replace('{Resolution}', this.exampleVR.resolution)

      const vrFormat = this.exampleVR.vrFormat ? '_' + this.exampleVR.vrFormat : ''
      result = result.replace('{VRFormat}', vrFormat)

      const alpha = this.exampleVR.alpha ? '_' + this.exampleVR.alpha : ''
      result = result.replace('{Alpha}', alpha)

      result = result.replace('{SceneID}', this.exampleScene.sceneId)
      result = result.replace('{ext}', 'mp4')

      if (this.linuxFriendly) {
        result = result.replace(/ /g, '_')
        result = result.replace(/:/g, '_')
        result = result.replace(/-/g, '_')
        result = result.replace(/['"$&|;()[\]{}*?#!\\<>`]/g, '')
        result = result.replace(/_+/g, '_')
        result = result.replace(/^_|_$/g, '')
      } else {
        result = result.replace(/:/g, '\uA789')
      }

      return result
    }
  },
  watch: {
    linuxFriendly (newValue) {
      this.actorSeparator = newValue ? '+' : ', '
    }
  },
  mounted () {
    this.loadSettings()
    this.loadFilters()
    this.loadFiles()
  },
  methods: {
    // Settings methods
    async loadSettings () {
      try {
        const data = await ky.get('/api/options/file-rename').json()
        this.pattern = data.pattern || '{Site} - {Title} - {Actors} - {Resolution}{VRFormat}{Alpha}.{ext}'
        this.actorSeparator = data.actorSeparator || ', '
        this.maxActors = data.maxActors || 3
        this.linuxFriendly = data.linuxFriendly || false
      } catch (err) {
        console.error('Failed to load settings:', err)
      }
    },
    async saveSettings () {
      this.isSaving = true
      try {
        await ky.put('/api/options/file-rename', {
          json: {
            pattern: this.pattern,
            actorSeparator: this.actorSeparator,
            maxActors: this.maxActors,
            linuxFriendly: this.linuxFriendly
          }
        })
        this.$buefy.toast.open({ message: this.$t('Settings saved'), type: 'is-success' })
        this.loadFiles()
      } catch (err) {
        this.$buefy.toast.open({ message: this.$t('Failed to save settings'), type: 'is-danger' })
      } finally {
        this.isSaving = false
      }
    },
    resetToDefaults () {
      this.pattern = '{Site} - {Title} - {Actors} - {Resolution}{VRFormat}{Alpha}.{ext}'
      this.actorSeparator = ', '
      this.maxActors = 3
      this.linuxFriendly = false
    },
    insertToken (token) {
      const input = this.$refs.patternInput.$el.querySelector('input')
      if (input) {
        const start = input.selectionStart || 0
        const end = input.selectionEnd || 0
        this.pattern = this.pattern.substring(0, start) + token + this.pattern.substring(end)
        this.$nextTick(() => {
          const newPos = start + token.length
          input.setSelectionRange(newPos, newPos)
          input.focus()
        })
      } else {
        this.pattern += token
      }
    },
    // Filter methods
    getFilteredSites (text) {
      this.filteredSites = this.availableSites.filter(site =>
        site.toLowerCase().includes(text.toLowerCase())
      )
    },
    getFilteredCast (text) {
      this.filteredCast = this.availableCast.filter(actor =>
        actor.toLowerCase().includes(text.toLowerCase())
      )
    },
    // Data loading
    async loadFilters () {
      try {
        const data = await ky.get('/api/files/manager/filters').json()
        this.availableSites = data.sites || []
        this.availableCast = data.cast || []
        this.availableSavedSearches = data.savedSearches || []
        this.availableResolutions = data.resolutions || []
        this.filteredSites = this.availableSites
        this.filteredCast = this.availableCast
        this.stats = data.stats
      } catch (err) {
        console.error('Failed to load filters:', err)
      }
    },
    async loadFiles () {
      this.isLoading = true
      this.selectedFiles = []
      try {
        const payload = {
          limit: this.perPage,
          offset: (this.currentPage - 1) * this.perPage,
          renamedStatus: this.filters.renamedStatus
        }
        if (this.filters.savedSearchId) payload.savedSearchId = this.filters.savedSearchId
        if (this.filters.sites.length > 0) payload.sites = this.filters.sites
        if (this.filters.cast.length > 0) payload.cast = this.filters.cast
        if (this.filters.resolution) payload.resolutions = [this.filters.resolution]

        const data = await ky.post('/api/files/manager/list', { json: payload }).json()
        this.files = data.files || []
        this.totalFiles = data.total
      } catch (err) {
        console.error('Failed to load files:', err)
        this.$buefy.toast.open({ message: this.$t('Failed to load files'), type: 'is-danger' })
      } finally {
        this.isLoading = false
      }
    },
    onPageChange (page) {
      this.currentPage = page
      this.loadFiles()
    },
    // Rename methods
    async renameSingle (item) {
      this.renamingFileId = item.file.id
      try {
        await ky.post('/api/files/rename', { json: { file_id: item.file.id } }).json()
        this.$buefy.toast.open({ message: this.$t('File renamed successfully'), type: 'is-success' })
        this.loadFiles()
        this.loadFilters()
      } catch (err) {
        this.$buefy.toast.open({ message: this.$t('Failed to rename file'), type: 'is-danger' })
      } finally {
        this.renamingFileId = null
      }
    },
    async renameSelected () {
      if (this.selectedFiles.length === 0) return
      this.isRenaming = true
      try {
        const fileIds = this.selectedFiles.map(f => f.file.id)
        await ky.post('/api/files/rename-batch', { json: { file_ids: fileIds } }).json()
        this.$buefy.toast.open({ message: this.$t('Files renamed successfully'), type: 'is-success' })
        this.selectedFiles = []
        this.loadFiles()
        this.loadFilters()
      } catch (err) {
        this.$buefy.toast.open({ message: this.$t('Failed to rename files'), type: 'is-danger' })
      } finally {
        this.isRenaming = false
      }
    }
  }
}
</script>

<style scoped>
.filename {
  font-family: monospace;
  font-size: 0.85rem;
  word-break: break-all;
}
.preview-output {
  color: #3273dc;
  word-break: break-all;
}
.tokens-help {
  margin-top: 1rem;
}
.tokens-help .tags {
  flex-wrap: wrap;
}
.tokens-help .tag {
  margin-bottom: 0.5rem;
}
code.is-block {
  display: block;
  padding: 0.5rem;
  background-color: #f5f5f5;
  border-radius: 4px;
  word-break: break-all;
}
</style>
