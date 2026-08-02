import './style.css';

// 应用状态
const state = {
    currentPage: 'task',
    tasks: [],
    currentTask: null,
    messages: [],
    todos: [],
    workingFiles: [],
    isProcessing: false
};

// 初始化
document.addEventListener('DOMContentLoaded', () => {
    initNavigation();
    loadSampleData();
    renderTaskList();
    setupInputHandlers();
});

// 导航切换
function initNavigation() {
    document.querySelectorAll('.nav-item').forEach(item => {
        item.addEventListener('click', () => {
            document.querySelectorAll('.nav-item').forEach(i => i.classList.remove('active'));
            item.classList.add('active');
            
            const page = item.dataset.page;
            switchPage(page);
        });
    });
}

function switchPage(page) {
    state.currentPage = page;
    document.querySelectorAll('.page').forEach(p => p.classList.add('hidden'));
    
    if (page === 'task') {
        document.getElementById('page-task').classList.remove('hidden');
    }
}

// 加载示例数据
function loadSampleData() {
    state.tasks = [
        {
            id: 1,
            title: '复制太极 code 无登录',
            subtitle: '内容由 AI 生成',
            time: '1分钟前',
            steps: 3
        },
        {
            id: 2,
            title: '排查并修复开发闪退',
            subtitle: '内容由 AI 生成',
            time: '2小时前',
            steps: 6
        },
        {
            id: 3,
            title: '生成桌面版安装包',
            subtitle: '内容由 AI 生成',
            time: '昨天',
            steps: 6
        }
    ];
    
    state.todos = [
        { id: 1, text: '修复tui.go编译错误并完成Qoder风格迁移', completed: true },
        { id: 2, text: '编译验证太极Code', completed: true },
        { id: 3, text: '运行测试套件', completed: true },
        { id: 4, text: '重建安装包', completed: true }
    ];
    
    state.workingFiles = [
        { name: 'project.go', icon: '📄' },
        { name: 'cache.go', icon: '' },
        { name: 'subagent.go', icon: '📄' },
        { name: 'client.go', icon: '📄' },
        { name: 'agent.go', icon: '📄' },
        { name: 'main.go', icon: '📄' },
        { name: 'tui.go', icon: '📄' },
        { name: 'web.go', icon: '📄' },
        { name: 'background_bash.go', icon: '📄' },
        { name: 'todo.go', icon: '📄' },
        { name: 'hooks.go', icon: '' }
    ];
}

// 渲染任务列表
function renderTaskList() {
    const taskList = document.getElementById('task-list');
    taskList.innerHTML = state.tasks.map(task => `
        <div class="task-item" onclick="openTask(${task.id})">
            <div class="task-item-title">${task.title}</div>
            <div class="task-item-meta">
                <span> ${task.time}</span>
                <span>📋 ${task.steps} 个步骤</span>
            </div>
        </div>
    `).join('');
}

// 打开任务
function openTask(taskId) {
    const task = state.tasks.find(t => t.id === taskId);
    if (!task) return;
    
    state.currentTask = task;
    document.getElementById('task-title').textContent = task.title;
    document.getElementById('task-subtitle').textContent = task.subtitle;
    
    document.getElementById('page-task').classList.add('hidden');
    document.getElementById('page-task-detail').classList.remove('hidden');
    
    // 加载示例对话
    loadSampleConversation();
    renderTodos();
    renderWorkingFiles();
}

// 返回任务列表
function backToTaskList() {
    document.getElementById('page-task-detail').classList.add('hidden');
    document.getElementById('page-task').classList.remove('hidden');
    state.currentTask = null;
}

// 加载示例对话
function loadSampleConversation() {
    state.messages = [
        {
            type: 'user',
            content: '当前代码把整个背景填了白色，需要改成透明。重写 makeicon.go，让圆形外部完全透明，边缘做抗锯齿：',
            time: '21:28'
        },
        {
            type: 'assistant',
            content: '图标已改为透明通道版本：<br><br>• 圆形外部完全透明（alpha=0）<br>• 边缘做了 0.5px 抗锯齿过渡<br>• 32位 RGBA 编码，PNG 压缩嵌入 ICO<br>• 桌面快捷方式和安装程序图标都会显示透明背景的太极图',
            time: '21:28',
            steps: [
                { title: '查看 3 个步骤', content: '当前代码把整个背景填了白色，需要改成透明。重写 makeicon.go，让圆形外部完全透明，边缘做抗锯齿：' },
                { title: '查看 6 个步骤', content: '图标生成成功，5.8K。重新编译安装包：' },
                { title: '查看 6 个步骤', content: '完成。图标已改为透明通道版本：' }
            ]
        }
    ];
    
    renderMessages();
}

// 渲染消息
function renderMessages() {
    const area = document.getElementById('conversation-area');
    area.innerHTML = state.messages.map(msg => {
        const isUser = msg.type === 'user';
        let contentHtml = `<div class="message-content ${isUser ? 'user-msg' : 'assistant-msg'}">${msg.content}</div>`;
        
        // 添加工具调用卡片
        if (msg.toolCalls) {
            contentHtml += msg.toolCalls.map(tc => `
                <div class="tool-call-card">
                    <div class="tool-call-header">
                        <span class="tool-call-icon">🔧</span>
                        <span class="tool-call-name">${tc.name}</span>
                    </div>
                    <div class="tool-call-args">${tc.args}</div>
                </div>
            `).join('');
        }
        
        // 添加步骤折叠
        if (msg.steps) {
            contentHtml += msg.steps.map((step, idx) => `
                <div class="step-collapse">
                    <div class="step-collapse-header" onclick="toggleStep(this)">
                        <span>◎</span>
                        <span>${step.title}</span>
                    </div>
                    <div class="step-collapse-content" style="display: none;">
                        ${step.content}
                    </div>
                </div>
            `).join('');
        }
        
        // 添加文件附件
        if (msg.files) {
            contentHtml += msg.files.map(f => `
                <div class="file-attachment">
                    <span class="file-icon">📄</span>
                    <div class="file-info">
                        <div class="file-name">${f.name}</div>
                        <div class="file-path">${f.path}</div>
                    </div>
                </div>
            `).join('');
        }
        
        return `
            <div class="message">
                <div class="message-header">
                    <div class="message-avatar ${isUser ? 'user' : 'assistant'}">
                        ${isUser ? '👤' : '🤖'}
                    </div>
                    <span class="message-sender">${isUser ? '用户' : '助手'}</span>
                    <span class="message-time">${msg.time}</span>
                </div>
                ${contentHtml}
            </div>
        `;
    }).join('');
    
    // 滚动到底部
    area.scrollTop = area.scrollHeight;
}

// 渲染待办事项
function renderTodos() {
    const todoList = document.getElementById('todo-list');
    todoList.innerHTML = state.todos.map(todo => `
        <div class="todo-item ${todo.completed ? 'completed' : ''}">
            <div class="todo-icon">${todo.completed ? '✓' : ''}</div>
            <span>${todo.text}</span>
        </div>
    `).join('');
}

// 渲染工作文件
function renderWorkingFiles() {
    const filesList = document.getElementById('working-files');
    filesList.innerHTML = state.workingFiles.map(file => `
        <div class="file-item">
            <span class="file-item-icon">${file.icon}</span>
            <span class="file-item-name">${file.name}</span>
        </div>
    `).join('');
}

// 切换步骤折叠
function toggleStep(header) {
    const content = header.nextElementSibling;
    content.style.display = content.style.display === 'none' ? 'block' : 'none';
}

// 设置输入处理器
function setupInputHandlers() {
    const input = document.getElementById('message-input');
    
    // Enter 发送（Shift+Enter 换行）
    input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendMessage();
        }
    });
    
    // 自动调整高度
    input.addEventListener('input', () => {
        input.style.height = 'auto';
        input.style.height = Math.min(input.scrollHeight, 200) + 'px';
    });
}

// 发送消息
async function sendMessage() {
    const input = document.getElementById('message-input');
    const content = input.value.trim();
    
    if (!content || state.isProcessing) return;
    
    // 添加用户消息
    state.messages.push({
        type: 'user',
        content: content,
        time: new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
    });
    
    input.value = '';
    input.style.height = 'auto';
    renderMessages();
    
    // 模拟 AI 响应
    state.isProcessing = true;
    updateSendButton();
    
    // 显示加载状态
    const loadingMsg = {
        type: 'assistant',
        content: '<div class="loading"></div>',
        time: new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
    };
    state.messages.push(loadingMsg);
    renderMessages();
    
    // 模拟延迟
    setTimeout(() => {
        // 移除加载消息
        state.messages.pop();
        
        // 添加 AI 响应
        state.messages.push({
            type: 'assistant',
            content: '收到您的消息。这是一个演示界面，实际功能需要连接太极 Code 后端。<br><br>当前已实现：<br>• 三栏布局（侧边栏 + 主面板 + 右侧监控）<br>• 任务列表和详情切换<br>• 对话消息显示<br>• 待办事项和工作文件展示<br>• 工具调用卡片和步骤折叠',
            time: new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
        });
        
        state.isProcessing = false;
        updateSendButton();
        renderMessages();
    }, 1500);
}

// 更新发送按钮状态
function updateSendButton() {
    const btn = document.querySelector('.btn-send');
    btn.disabled = state.isProcessing;
}

// 新建任务
function newTask() {
    document.getElementById('page-task').classList.add('hidden');
    document.getElementById('page-task-detail').classList.remove('hidden');
    document.getElementById('task-title').textContent = '新任务';
    document.getElementById('task-subtitle').textContent = '开始描述你的任务';
    state.messages = [];
    renderMessages();
    document.getElementById('message-input').focus();
}

// 暴露到全局
window.openTask = openTask;
window.backToTaskList = backToTaskList;
window.sendMessage = sendMessage;
window.newTask = newTask;
window.toggleStep = toggleStep;
