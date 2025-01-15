import { useEffect, useState } from 'react'
import { Button, Card, Flex, List, ListItem, Text, Title } from '@tremor/react'
import { ChevronRightIcon } from '@heroicons/react/24/outline'
import { useAuthApiV1WorkspaceRoleBindingsList } from '../../../api/auth.gen'
import Spinner from '../../../components/Spinner'
import MemberDetails from './MemberDetails'
import MemberInvite from './MemberInvite'
import Notification from '../../../components/Notification'
import {
    dateTimeDisplay,
    shortDateTimeDisplay,
} from '../../../utilities/dateDisplay'
import TopHeader from '../../../components/Layout/Header'
import { useSearchParams } from 'react-router-dom'
import { Alert, Box, Header, Link, Modal, SpaceBetween, Table, Toggle } from '@cloudscape-design/components'
import KButton from '@cloudscape-design/components/button'
import SettingsConnectors from '../Connectors'
const fixRole = (role: string) => {
    switch (role) {
        case 'admin':
            return 'Admin'
        case 'editor':
            return 'Editor'
        case 'viewer':
            return 'Viewer'
        default:
            return role
    }
}

export default function SettingsMembers() {
    const [drawerOpen, setDrawerOpen] = useState<boolean>(false)
    const [drawerParam, setDrawerParam] = useState<number | string>(0)
    // const [showSSO, setShowSSO] = useState<boolean>(false)

    const {
        response,
        isLoading,
        sendNow: refreshRoleBindings,
    } = useAuthApiV1WorkspaceRoleBindingsList()

    const userDetail = (userId: number) => {
        setDrawerParam(userId)
        setDrawerOpen(true)
    }
    const openInviteMember = () => {
        setDrawerParam('openInviteMember')
        setDrawerOpen(true)
    }
    const [searchParams, setSearchParams] = useSearchParams()
    const [show, setShow] = useState<boolean>(false)

    useEffect(() => {
        const tab_id = searchParams.get('action')
        switch (tab_id) {
            case 'invite':
                setDrawerParam('openInviteMember')
                setDrawerOpen(true)
                break

            default:
                break
        }
    }, [searchParams])
    return isLoading ? (
        <Flex justifyContent="center" className="mt-56">
            <Spinner />
        </Flex>
    ) : (
        <>
            {/* <TopHeader /> */}
            <Modal
                visible={drawerOpen}
                header={
                    drawerParam === 'openInviteMember'
                        ? 'Invite New Members'
                        : response?.find((item) => item?.id === drawerParam)
                              ?.email
                }
                onDismiss={() => {
                    setDrawerOpen(false)
                }}
            >
                {drawerParam === 'openInviteMember' ? (
                    <MemberInvite
                        close={(refresh: boolean) => {
                            setDrawerOpen(false)
                            if (refresh) {
                                refreshRoleBindings()
                            }
                        }}
                    />
                ) : (
                    <MemberDetails
                        user={response?.find((item) => item.id === drawerParam)}
                        close={() => {
                            setDrawerOpen(false)
                            refreshRoleBindings()
                        }}
                    />
                )}
            </Modal>
            {/* <a
                className=" cursor-pointer"
                target="__blank"
                href="https://docs.opengovernance.io/oss/getting-started/configure-sso"
            >
                <Alert statusIconAriaLabel="Info" header=" SSO Configuration">
                    Enable SSO in OpenGovernance using OIDC. Integrate with
                    Auth0, AzureAD, Google, and more.
                    <Link
                        href="  https://docs.opengovernance.io/oss/getting-started/configure-sso"
                        external={true}
                    >
                        Click here.
                    </Link>
                </Alert>
            </a> */}
            <Table
                className="mt-2 mb-5"
                onRowClick={(event) => {
                    const row = event.detail.item
                    if (row.id) {
                        userDetail(row.id)
                    }
                }}
                columnDefinitions={[
                    {
                        id: 'email',
                        header: 'Email',
                        cell: (item: any) => item.email,
                    },
                    {
                        id: 'created_at',
                        header: 'Member Since',
                        cell: (item: any) =>
                            item.created_at
                                ? dateTimeDisplay(item.created_at)
                                : 'Never',
                    },
                    {
                        id: 'last_activity',
                        header: 'Last Activity',
                        cell: (item: any) =>
                            item.last_activity
                                ? dateTimeDisplay(item.last_activity)
                                : 'Never',
                    },
                    {
                        id: 'role',
                        header: 'Role',
                        cell: (item: any) => (
                            <Flex
                                justifyContent="start"
                                className="truncate w-full"
                            >
                                <div className="truncate p-1">
                                    <Text className="truncate font-medium text-gray-800">
                                        {fixRole(item.role_name || '')}
                                    </Text>
                                    {/* <Text className="truncate text-xs text-gray-400">
                                        {(item.scopedConnectionIDs?.length ||
                                            0) === 0
                                            ? 'All accounts'
                                            : `${item.scopedConnectionIDs?.length} accounts`}
                                    </Text> */}
                                </div>
                            </Flex>
                        ),
                    },
                    {
                        id: 'active',
                        header: 'Status',
                        cell: (item: any) => (
                            <Flex
                                justifyContent="start"
                                className="truncate w-full"
                            >
                                {item?.is_active ? 'Active' : 'Inactive'}
                            </Flex>
                        ),
                    },
                ]}
                columnDisplay={[
                    { id: 'email', visible: true },
                    { id: 'created_at', visible: true },
                    { id: 'last_activity', visible: true },
                    { id: 'role', visible: true },
                    { id: 'active', visible: true },

                    // { id: 'action', visible: true },
                ]}
                loading={isLoading}
                // @ts-ignore
                items={response}
                empty={
                    <Box
                        margin={{ vertical: 'xs' }}
                        textAlign="center"
                        color="inherit"
                    >
                        <SpaceBetween size="m">
                            <b>No resources</b>
                        </SpaceBetween>
                    </Box>
                }
                header={
                    <Header
                        actions={
                            <>
                                <KButton
                                    className="float-right"
                                    variant="primary"
                                    onClick={() => {
                                        openInviteMember()
                                    }}
                                >
                                    Add Users
                                </KButton>
                            </>
                        }
                        className="w-full"
                    >
                        Members{' '}
                    </Header>
                }
            />
            {/* <Card key="summary" className="mt-4">
                <Flex>
                    <Title className="font-semibold">All members</Title>
                </Flex>

                <List className="mt-4">
                    <ListItem>
                        <Text className="w-1/3 text-start">Member</Text>
                        <Text className="w-1/3 text-center">Member Since</Text>
                        <Text className="w-1/3 text-center">Last Activity</Text>
                        <Text className="w-1/3 text-end mr-5">Role</Text>
                    </ListItem>
                    {response?.map((item) => (
                        <ListItem
                            key={item.userName}
                            onClick={() => {
                                if (item.userId) {
                                    userDetail(item.userId)
                                }
                            }}
                            className="cursor-pointer"
                        >
                            <Flex
                                justifyContent="between"
                                className="truncate space-x-4 w-1/3"
                            >
                                <div className="truncate p-1">
                                    <Text className="truncate font-medium text-gray-800">
                                        {item.userName}
                                    </Text>
                                    <Text className="truncate text-xs text-gray-400">
                                        {item.email}
                                    </Text>
                                </div>
                            </Flex>
                            <Text className="w-1/3 text-center">
                                {item.createdAt
                                    ? dateTimeDisplay(item.createdAt)
                                    : 'Never'}
                            </Text>
                            <Text className="w-1/3 text-center">
                                {item.lastActivity
                                    ? dateTimeDisplay(item.lastActivity)
                                    : 'Never'}
                            </Text>
                            <Flex
                                flexDirection="row"
                                className="w-1/3"
                                alignItems="center"
                                justifyContent="end"
                            >
                                <Flex
                                    justifyContent="end"
                                    className="truncate w-full"
                                >
                                    <div className="truncate p-1">
                                        <Text className="truncate font-medium text-gray-800">
                                            {fixRole(item.roleName || '')}
                                        </Text>
                                        <Text className="truncate text-xs text-gray-400">
                                            {(item.scopedConnectionIDs
                                                ?.length || 0) === 0
                                                ? 'All accounts'
                                                : `${item.scopedConnectionIDs?.length} accounts`}
                                        </Text>
                                    </div>
                                </Flex>
                                <ChevronRightIcon className="h-6 w-6 shrink-0" />
                            </Flex>
                        </ListItem>
                    ))}
                </List>
            </Card> */}
            {/* <Toggle checked={showSSO} onChange={() => setShowSSO(!showSSO)}>
                Show SSO Providers
            </Toggle>
            {showSSO && <SettingsConnectors />} */}
        </>
    )
}