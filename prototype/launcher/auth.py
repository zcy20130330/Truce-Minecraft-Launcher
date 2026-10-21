# -*- coding: utf-8 -*-
"""登录（对标 PCL ModLaunch.vb McLogin* 简化版）。

原型阶段实现：离线登录（UUID 与 Java 端一致，服务器可识别）。
微软正版 OAuth 六步留占位，后续接入。
"""
import hashlib
import uuid


def offline_login(username):
    """离线登录：UUID 采用 Java 版 UUID.nameUUIDFromBytes("OfflinePlayer:"+name) 算法。"""
    raw = hashlib.md5(("OfflinePlayer:" + username).encode("utf-8")).digest()
    b = bytearray(raw)
    b[6] = (b[6] & 0x0F) | 0x30  # version 3
    b[8] = (b[8] & 0x3F) | 0x80  # IETF variant
    uid = str(uuid.UUID(bytes=bytes(b)))
    return {
        "name": username,
        "uuid": uid,
        "access_token": uid,   # 离线模式 PCL 中 AccessToken = Uuid
        "user_type": "legacy",
    }


def ms_login_hint():
    """微软正版登录：原型占位。"""
    raise NotImplementedError("微软正版登录（MS OAuth 6 步）将在下个迭代接入，原型阶段请使用离线登录。")
