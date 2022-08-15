// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.
import React from 'react'

import {render, waitFor} from "@testing-library/react"

import '@testing-library/jest-dom'


import {Provider as ReduxProvider} from "react-redux"

import {MemoryRouter} from "react-router-dom"

import userEvent from "@testing-library/user-event"

import {mocked} from "jest-mock"

import {mockStateStore, wrapDNDIntl} from "../../testUtils"
import {TestBlockFactory} from "../../test/testBlockFactory"
import {Board} from "../../blocks/board"

import client from "../../octoClient"

import ViewLimitModalWrapper from "./viewLimitDialogWrapper"

jest.mock('../../octoClient')
const mockedOctoClient = mocked(client, true)

describe('components/viewLimitDialog/ViewL]imitDialog', () => {
    const board: Board = {
        ...TestBlockFactory.createBoard(),
        id: 'board_id_1',
    }

    const state = {
        users: {
            me: {
                id: 'user_id_1',
                username: 'Michael Scott',
                roles: 'system_user',
            },
        },
        boards: {
            boards: {
                [board.id]: board,
            },
            current: board.id,
        }
    }

    const store = mockStateStore([], state)
    beforeEach(() => {
        jest.clearAllMocks()
    })

    test('show view limit dialog', async () => {
        const handleOnClose = jest.fn()
        mockedOctoClient.notifyAdminUpgrade.mockResolvedValue()

        const {container} = render(wrapDNDIntl(
            <ReduxProvider store={store}>
                <ViewLimitModalWrapper
                    onClose={handleOnClose}
                    show={true}
                />
            </ReduxProvider>
        ), {wrapper: MemoryRouter})
        expect(container).toMatchSnapshot()
    })

    test('show notify admin confirmation msg', async () => {
        const handleOnClose = jest.fn()
        mockedOctoClient.notifyAdminUpgrade.mockResolvedValue()

        const {container} = render(wrapDNDIntl(
            <ReduxProvider store={store}>
                <ViewLimitModalWrapper
                    onClose={handleOnClose}
                    show={true}
                />
            </ReduxProvider>
        ), {wrapper: MemoryRouter})

        const notifyBtn = container.querySelector('button.primaryAction')
        expect(notifyBtn).toBeDefined()
        expect(notifyBtn).not.toBeNull()
        expect(notifyBtn!.textContent).toBe('Notify Admin')
        userEvent.click(notifyBtn as Element)
        await waitFor(() => expect(container.querySelector('.ViewLimitSuccessNotify')).toBeInTheDocument())
        expect(container).toMatchSnapshot()
    })
})
