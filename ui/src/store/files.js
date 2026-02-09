import ky from 'ky'

const state = {
  isLoading: false,
  items: [],
  selectedFiles: [],
  filters: {
    sort: '',
    state: 'unmatched',
    createdDate: [],
    resolutions: [],
    framerates: [],
    bitrates: [],
    filename: ''
  }
}

const getters = {
  prevFile: (state) => (currentFile) => {
    if (state.items.length <= 1) {
      return null
    }

    const currentIndex = state.items.findIndex(item => item.id === currentFile.id)
    if (currentIndex === 0) {
      return state.items[state.items.length - 1]
    }
    return state.items[currentIndex - 1]
  },
  nextFile: (state) => (currentFile) => {
    if (state.items.length <= 1) {
      return null
    }

    const currentIndex = state.items.findIndex(item => item.id === currentFile.id)
    if (currentIndex === state.items.length - 1) {
      return state.items[0]
    }
    return state.items[currentIndex + 1]
  }
}

const mutations = {
  setSelectedFiles (state, files) {
    state.selectedFiles = files
  },
  clearSelectedFiles (state) {
    state.selectedFiles = []
  },
  toggleFileSelection (state, file) {
    const index = state.selectedFiles.findIndex(f => f.id === file.id)
    if (index === -1) {
      state.selectedFiles.push(file)
    } else {
      state.selectedFiles.splice(index, 1)
    }
  }
}

const actions = {
  load ({ state }, params) {
    state.isLoading = true
    ky.post('/api/files/list', { json: state.filters })
      .json()
      .then(data => {
        state.items = data
        state.isLoading = false
      })
  },
  async previewRename ({ commit }, fileId) {
    const response = await ky.post('/api/files/rename/preview', {
      json: { file_id: fileId }
    }).json()
    return response
  },
  async renameFile ({ dispatch }, fileId) {
    const response = await ky.post('/api/files/rename', {
      json: { file_id: fileId }
    }).json()
    dispatch('load')
    return response
  },
  async previewBatchRename ({ commit }, fileIds) {
    const response = await ky.post('/api/files/rename/preview-batch', {
      json: { file_ids: fileIds }
    }).json()
    return response
  },
  async batchRename ({ dispatch, commit }, fileIds) {
    const response = await ky.post('/api/files/rename-batch', {
      json: { file_ids: fileIds }
    }).json()
    commit('clearSelectedFiles')
    dispatch('load')
    return response
  }
}

export default {
  namespaced: true,
  state,
  getters,
  mutations,
  actions
}
