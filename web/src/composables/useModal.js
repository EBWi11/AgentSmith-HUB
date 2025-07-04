import { ref, onBeforeUnmount } from 'vue'

/**
 * 通用模态框管理composable
 * 用于减少重复的模态框处理代码
 * 仅供内部使用，不直接导出
 */
function useModal() {
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
 * 仅供内部使用，不直接导出
 */
function useEscapeKey(callback) {
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