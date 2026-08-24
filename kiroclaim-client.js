/**
 * KiroClaim Node.js 客户端
 * 用于从外部系统向 KiroClaim 导入账号
 */

const axios = require('axios');

class KiroClaimClient {
    /**
     * 初始化客户端
     * @param {string} baseURL - KiroClaim 服务器地址，如: http://localhost:8080
     * @param {string} username - 管理员用户名
     * @param {string} password - 管理员密码
     * @param {number} timeout - 请求超时时间（毫秒）
     */
    constructor(baseURL, username, password, timeout = 30000) {
        this.baseURL = baseURL.replace(/\/$/, '');
        this.auth = {
            username,
            password
        };
        this.timeout = timeout;
        
        // 创建 axios 实例
        this.client = axios.create({
            baseURL: this.baseURL,
            timeout: this.timeout,
            auth: this.auth,
            headers: {
                'Content-Type': 'application/json'
            }
        });
    }

    /**
     * 导入账号
     * @param {Array} accounts - 账号列表，每个账号至少包含 refreshToken
     * @param {boolean} waitCompletion - 是否等待导入完成
     * @param {number} maxWait - 最大等待时间（秒）
     * @returns {Promise<Object>} 导入结果
     */
    async importAccounts(accounts, waitCompletion = false, maxWait = 60) {
        try {
            // 发送导入请求
            const response = await this.client.post('/admin/accounts/import', accounts);
            
            if (response.data.code !== 0) {
                return {
                    success: false,
                    error: response.data.message || '未知错误'
                };
            }
            
            const { taskId, total } = response.data.data;
            
            console.log(`导入任务已创建: taskId=${taskId}, total=${total}`);
            
            // 如果不等待，直接返回
            if (!waitCompletion) {
                return {
                    success: true,
                    taskId,
                    total,
                    status: 'processing'
                };
            }
            
            // 等待导入完成
            return await this._waitForImportCompletion(taskId, maxWait);
            
        } catch (error) {
            console.error('导入请求失败:', error.message);
            return {
                success: false,
                error: error.message
            };
        }
    }

    /**
     * 导入单个账号
     * @param {Object} account - 账号信息
     * @param {string} account.refreshToken - AWS 刷新令牌（必需）
     * @param {string} account.accessToken - 访问令牌
     * @param {string} account.clientId - 客户端 ID
     * @param {string} account.clientSecret - 客户端密钥
     * @param {string} account.provider - 提供商标识
     * @param {string} account.region - 区域
     * @param {boolean} waitCompletion - 是否等待导入完成
     * @returns {Promise<Object>} 导入结果
     */
    async importSingleAccount(account, waitCompletion = true) {
        if (!account.refreshToken) {
            return {
                success: false,
                error: 'refreshToken 是必需的'
            };
        }
        
        return await this.importAccounts([account], waitCompletion);
    }

    /**
     * 查询导入状态
     * @param {string} taskId - 任务 ID
     * @returns {Promise<Object>} 任务状态
     */
    async getImportStatus(taskId) {
        try {
            const response = await this.client.get(`/admin/accounts/import/status/${taskId}`);
            
            if (response.data.code !== 0) {
                return {
                    success: false,
                    error: response.data.message || '未知错误'
                };
            }
            
            return {
                success: true,
                ...response.data.data
            };
            
        } catch (error) {
            if (error.response && error.response.status === 404) {
                return {
                    success: false,
                    error: '任务不存在'
                };
            }
            
            console.error('查询状态失败:', error.message);
            return {
                success: false,
                error: error.message
            };
        }
    }

    /**
     * 等待导入完成
     * @private
     * @param {string} taskId - 任务 ID
     * @param {number} maxWait - 最大等待时间（秒）
     * @returns {Promise<Object>} 最终状态
     */
    async _waitForImportCompletion(taskId, maxWait) {
        const startTime = Date.now();
        
        while ((Date.now() - startTime) / 1000 < maxWait) {
            const status = await this.getImportStatus(taskId);
            
            if (!status.success) {
                return status;
            }
            
            if (status.status === 'completed') {
                console.log(`导入完成: imported=${status.imported}, skipped=${status.skippedDup + status.skippedBad}`);
                return {
                    success: true,
                    taskId,
                    status: 'completed',
                    total: status.total,
                    imported: status.imported,
                    skippedDup: status.skippedDup,
                    skippedBad: status.skippedBad,
                    badDetails: status.badDetails || []
                };
            }
            
            if (status.status === 'failed') {
                console.error('导入任务失败');
                return {
                    success: false,
                    taskId,
                    status: 'failed',
                    error: '导入任务失败'
                };
            }
            
            // 仍在处理中，等待
            await new Promise(resolve => setTimeout(resolve, 1000));
        }
        
        // 超时
        console.warn(`导入超时，taskId=${taskId}`);
        return {
            success: false,
            taskId,
            error: `导入超时（>${maxWait}秒），请稍后查询状态`
        };
    }
}

// 导出
module.exports = KiroClaimClient;

// 使用示例
if (require.main === module) {
    (async () => {
        // 初始化客户端
        const client = new KiroClaimClient(
            'http://localhost:8080',
            'admin',
            'password'
        );
        
        // 示例 1: 导入单个账号
        console.log('='.repeat(50));
        console.log('示例 1: 导入单个账号');
        console.log('='.repeat(50));
        
        const singleResult = await client.importSingleAccount({
            refreshToken: 'aorAAAAAGsBe20EWyIQBTqsxtwHcjZZQy-16c5kfpMxZtjnL17s...',
            accessToken: 'aoaAAAAAGqK90AXEkG6ByWyENlWQxv4naFrACAMz4aAHkUYnEg-...',
            clientId: 'cSTBkueixbIuMka6jh95QnVzLWVhc3QtMQ',
            clientSecret: 'eyJraWQiOiJrZXktMTU2NDAyODA5OSIsImFsZyI6IkhTMzg0In0...',
            provider: 'idc',
            region: 'us-east-1'
        }, true);
        
        console.log('导入结果:', JSON.stringify(singleResult, null, 2));
        
        // 示例 2: 批量导入账号
        console.log('\n' + '='.repeat(50));
        console.log('示例 2: 批量导入账号');
        console.log('='.repeat(50));
        
        const accounts = [
            {
                refreshToken: 'aorAAAAA_account1...',
                accessToken: 'aoaAAAAA_account1...',
                clientId: 'cSTB_account1...',
                clientSecret: 'eyJr_account1...'
            },
            {
                refreshToken: 'aorAAAAA_account2...',
                accessToken: 'aoaAAAAA_account2...',
                clientId: 'cSTB_account2...',
                clientSecret: 'eyJr_account2...'
            }
        ];
        
        const batchResult = await client.importAccounts(accounts, true, 30);
        console.log('批量导入结果:', JSON.stringify(batchResult, null, 2));
        
        // 示例 3: 异步导入（不等待）
        console.log('\n' + '='.repeat(50));
        console.log('示例 3: 异步导入（不等待完成）');
        console.log('='.repeat(50));
        
        const asyncResult = await client.importAccounts(accounts, false);
        console.log('导入已提交:', JSON.stringify(asyncResult, null, 2));
        
        if (asyncResult.success) {
            const taskId = asyncResult.taskId;
            
            // 稍后查询状态
            console.log('\n稍后查询状态...');
            await new Promise(resolve => setTimeout(resolve, 3000));
            
            const status = await client.getImportStatus(taskId);
            console.log('任务状态:', JSON.stringify(status, null, 2));
        }
    })();
}
