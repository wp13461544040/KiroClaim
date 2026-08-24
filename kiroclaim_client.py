#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
KiroClaim Python 客户端
用于从外部系统向 KiroClaim 导入账号
"""

import requests
import time
import logging
from typing import Dict, List, Optional, Union

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class KiroClaimClient:
    """KiroClaim API 客户端"""
    
    def __init__(self, base_url: str, username: str, password: str, timeout: int = 30):
        """
        初始化客户端
        
        Args:
            base_url: KiroClaim 服务器地址，如: http://localhost:8080
            username: 管理员用户名
            password: 管理员密码
            timeout: 请求超时时间（秒）
        """
        self.base_url = base_url.rstrip('/')
        self.auth = (username, password)
        self.timeout = timeout
        self.session = requests.Session()
        self.session.auth = self.auth
        
    def import_accounts(
        self, 
        accounts: List[Dict[str, str]], 
        wait_completion: bool = False,
        max_wait: int = 60
    ) -> Dict:
        """
        导入账号
        
        Args:
            accounts: 账号列表，每个账号至少包含 refreshToken
            wait_completion: 是否等待导入完成
            max_wait: 最大等待时间（秒）
            
        Returns:
            导入结果字典
            
        Example:
            >>> client = KiroClaimClient('http://localhost:8080', 'admin', 'password')
            >>> accounts = [
            ...     {
            ...         'refreshToken': 'aor...',
            ...         'accessToken': 'aoa...',
            ...         'clientId': 'cST...',
            ...         'clientSecret': 'eyJ...'
            ...     }
            ... ]
            >>> result = client.import_accounts(accounts, wait_completion=True)
            >>> print(result)
        """
        try:
            # 发送导入请求
            response = self.session.post(
                f"{self.base_url}/admin/accounts/import",
                json=accounts,
                timeout=self.timeout
            )
            
            if response.status_code != 200:
                return {
                    'success': False,
                    'error': f'HTTP {response.status_code}: {response.text}'
                }
            
            result = response.json()
            
            if result.get('code') != 0:
                return {
                    'success': False,
                    'error': result.get('message', '未知错误')
                }
            
            task_id = result['data']['taskId']
            total = result['data']['total']
            
            logger.info(f"导入任务已创建: taskId={task_id}, total={total}")
            
            # 如果不等待，直接返回
            if not wait_completion:
                return {
                    'success': True,
                    'taskId': task_id,
                    'total': total,
                    'status': 'processing'
                }
            
            # 等待导入完成
            return self._wait_for_import_completion(task_id, max_wait)
            
        except requests.RequestException as e:
            logger.error(f"导入请求失败: {e}")
            return {
                'success': False,
                'error': str(e)
            }
    
    def import_single_account(
        self, 
        refresh_token: str,
        access_token: Optional[str] = None,
        client_id: Optional[str] = None,
        client_secret: Optional[str] = None,
        provider: Optional[str] = None,
        region: Optional[str] = None,
        wait_completion: bool = True
    ) -> Dict:
        """
        导入单个账号
        
        Args:
            refresh_token: AWS 刷新令牌（必需）
            access_token: 访问令牌
            client_id: 客户端 ID
            client_secret: 客户端密钥
            provider: 提供商标识
            region: 区域
            wait_completion: 是否等待导入完成
            
        Returns:
            导入结果字典
        """
        account = {'refreshToken': refresh_token}
        
        if access_token:
            account['accessToken'] = access_token
        if client_id:
            account['clientId'] = client_id
        if client_secret:
            account['clientSecret'] = client_secret
        if provider:
            account['provider'] = provider
        if region:
            account['region'] = region
        
        return self.import_accounts([account], wait_completion=wait_completion)
    
    def get_import_status(self, task_id: str) -> Dict:
        """
        查询导入状态
        
        Args:
            task_id: 任务 ID
            
        Returns:
            任务状态字典
        """
        try:
            response = self.session.get(
                f"{self.base_url}/admin/accounts/import/status/{task_id}",
                timeout=self.timeout
            )
            
            if response.status_code == 404:
                return {
                    'success': False,
                    'error': '任务不存在'
                }
            
            if response.status_code != 200:
                return {
                    'success': False,
                    'error': f'HTTP {response.status_code}: {response.text}'
                }
            
            result = response.json()
            
            if result.get('code') != 0:
                return {
                    'success': False,
                    'error': result.get('message', '未知错误')
                }
            
            return {
                'success': True,
                **result['data']
            }
            
        except requests.RequestException as e:
            logger.error(f"查询状态失败: {e}")
            return {
                'success': False,
                'error': str(e)
            }
    
    def _wait_for_import_completion(self, task_id: str, max_wait: int) -> Dict:
        """
        等待导入完成
        
        Args:
            task_id: 任务 ID
            max_wait: 最大等待时间（秒）
            
        Returns:
            最终状态字典
        """
        start_time = time.time()
        
        while time.time() - start_time < max_wait:
            status = self.get_import_status(task_id)
            
            if not status['success']:
                return status
            
            task_status = status['status']
            
            if task_status == 'completed':
                logger.info(f"导入完成: imported={status['imported']}, "
                          f"skipped={status['skippedDup'] + status['skippedBad']}")
                return {
                    'success': True,
                    'taskId': task_id,
                    'status': 'completed',
                    'total': status['total'],
                    'imported': status['imported'],
                    'skippedDup': status['skippedDup'],
                    'skippedBad': status['skippedBad'],
                    'badDetails': status.get('badDetails', [])
                }
            
            elif task_status == 'failed':
                logger.error("导入任务失败")
                return {
                    'success': False,
                    'taskId': task_id,
                    'status': 'failed',
                    'error': '导入任务失败'
                }
            
            # 仍在处理中，等待
            time.sleep(1)
        
        # 超时
        logger.warning(f"导入超时，taskId={task_id}")
        return {
            'success': False,
            'taskId': task_id,
            'error': f'导入超时（>{max_wait}秒），请稍后查询状态'
        }


# 使用示例
if __name__ == '__main__':
    # 初始化客户端
    client = KiroClaimClient(
        base_url='http://localhost:8080',
        username='admin',
        password='password'
    )
    
    # 示例 1: 导入单个账号
    print("=" * 50)
    print("示例 1: 导入单个账号")
    print("=" * 50)
    
    result = client.import_single_account(
        refresh_token='aorAAAAAGsBe20EWyIQBTqsxtwHcjZZQy-16c5kfpMxZtjnL17s...',
        access_token='aoaAAAAAGqK90AXEkG6ByWyENlWQxv4naFrACAMz4aAHkUYnEg-...',
        client_id='cSTBkueixbIuMka6jh95QnVzLWVhc3QtMQ',
        client_secret='eyJraWQiOiJrZXktMTU2NDAyODA5OSIsImFsZyI6IkhTMzg0In0...',
        provider='idc',
        region='us-east-1',
        wait_completion=True
    )
    
    print(f"导入结果: {result}")
    
    # 示例 2: 批量导入账号
    print("\n" + "=" * 50)
    print("示例 2: 批量导入账号")
    print("=" * 50)
    
    accounts = [
        {
            'refreshToken': 'aorAAAAA_account1...',
            'accessToken': 'aoaAAAAA_account1...',
            'clientId': 'cSTB_account1...',
            'clientSecret': 'eyJr_account1...'
        },
        {
            'refreshToken': 'aorAAAAA_account2...',
            'accessToken': 'aoaAAAAA_account2...',
            'clientId': 'cSTB_account2...',
            'clientSecret': 'eyJr_account2...'
        }
    ]
    
    result = client.import_accounts(accounts, wait_completion=True, max_wait=30)
    print(f"批量导入结果: {result}")
    
    # 示例 3: 异步导入（不等待）
    print("\n" + "=" * 50)
    print("示例 3: 异步导入（不等待完成）")
    print("=" * 50)
    
    result = client.import_accounts(accounts, wait_completion=False)
    print(f"导入已提交: {result}")
    
    if result['success']:
        task_id = result['taskId']
        
        # 稍后查询状态
        print(f"\n稍后查询状态...")
        time.sleep(3)
        
        status = client.get_import_status(task_id)
        print(f"任务状态: {status}")
