import { ref, onBeforeUnmount } from 'vue'

/**
 * 通用模态框管理composable
 * 用于减少重复的模态框处理代码
 */
export function useModal() {
  const isOpen = ref(false)
  const data = ref(null)
  
  const open = (modalData = null) => {
    data.value = modalData
    isOpen.value = true
  }
  
  const close = () => {
    isOpen.value = false
    data.value = null
  }
  
  const toggle = () => {
    if (isOpen.value) {
      close()
    } else {
      open()
    }
  }
  
  return {
    isOpen,
    data,
    open,
    close,
    toggle
  }
}

/**
 * ESC键处理composable
 * 统一处理ESC键关闭模态框的逻辑
 */
export function useEscapeKey(callback) {
  const handleEscape = (event) => {
    if (event.key === 'Escape') {
      callback()
    }
  }
  
  const enableEscapeKey = () => {
    document.addEventListener('keydown', handleEscape)
  }
  
  const disableEscapeKey = () => {
    document.removeEventListener('keydown', handleEscape)
  }
  
  // 清理事件监听器
  onBeforeUnmount(() => {
    disableEscapeKey()
  })
  
  return {
    enableEscapeKey,
    disableEscapeKey
  }
}

/**
 * 多模态框管理composable
 * 用于管理多个命名模态框
 */
export function useMultiModal() {
  const modals = ref({})
  
  const isOpen = (name) => {
    return modals.value[name]?.isOpen || false
  }
  
  const getData = (name) => {
    return modals.value[name]?.data || null
  }
  
  const open = (name, data = null) => {
    if (!modals.value[name]) {
      modals.value[name] = { isOpen: false, data: null }
    }
    modals.value[name].isOpen = true
    modals.value[name].data = data
  }
  
  const close = (name) => {
    if (modals.value[name]) {
      modals.value[name].isOpen = false
      modals.value[name].data = null
    }
  }
  
  const closeAll = () => {
    Object.keys(modals.value).forEach(name => {
      close(name)
    })
  }
  
  const toggle = (name, data = null) => {
    if (isOpen(name)) {
      close(name)
    } else {
      open(name, data)
    }
  }
  
  return {
    modals,
    isOpen,
    getData,
    open,
    close,
    closeAll,
    toggle
  }
}

/**
 * 删除确认模态框composable
 * 专门用于处理删除确认对话框
 */
export function useDeleteConfirmModal() {
  const { isOpen, data, open, close } = useModal()
  const deleteText = ref('')
  const isDeleting = ref(false)
  
  const openDeleteModal = (item) => {
    deleteText.value = ''
    open(item)
  }
  
  const closeDeleteModal = () => {
    deleteText.value = ''
    isDeleting.value = false
    close()
  }
  
  const canDelete = () => {
    return deleteText.value.toLowerCase() === 'delete'
  }
  
  const confirmDelete = async (deleteFunction) => {
    if (!canDelete() || isDeleting.value) {
      return false
    }
    
    isDeleting.value = true
    try {
      await deleteFunction(data.value)
      closeDeleteModal()
      return true
    } catch (error) {
      throw error
    } finally {
      isDeleting.value = false
    }
  }
  
  return {
    isOpen,
    data,
    deleteText,
    isDeleting,
    openDeleteModal,
    closeDeleteModal,
    canDelete,
    confirmDelete
  }
}

/**
 * 测试模态框composable
 * 用于各种测试模态框（ruleset、plugin、project、output）
 */
export function useTestModal() {
  const testRuleset = useModal()
  const testPlugin = useModal()
  const testProject = useModal()
  const testOutput = useModal()
  
  const { enableEscapeKey, disableEscapeKey } = useEscapeKey(() => {
    closeAllTestModals()
  })
  
  const openTestRuleset = (data) => {
    testRuleset.open(data)
    enableEscapeKey()
  }
  
  const openTestPlugin = (data) => {
    testPlugin.open(data)
    enableEscapeKey()
  }
  
  const openTestProject = (data) => {
    testProject.open(data)
    enableEscapeKey()
  }
  
  const openTestOutput = (data) => {
    testOutput.open(data)
    enableEscapeKey()
  }
  
  const closeAllTestModals = () => {
    testRuleset.close()
    testPlugin.close()
    testProject.close()
    testOutput.close()
    disableEscapeKey()
  }
  
  return {
    testRuleset,
    testPlugin,
    testProject,
    testOutput,
    openTestRuleset,
    openTestPlugin,
    openTestProject,
    openTestOutput,
    closeAllTestModals
  }
} 